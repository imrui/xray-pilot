package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var dnsWake = make(chan struct{}, 1)

func wakeDNS() {
	select {
	case dnsWake <- struct{}{}:
	default:
	}
}

var dnsWorkStates = []string{entity.DNSPending, entity.DNSChecking}

func (s *DNSService) Run(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var workers sync.WaitGroup
	defer workers.Wait()
	running := sync.Map{}
	for {
		var ids []uint
		now := time.Now()
		err := repository.DB.WithContext(ctx).Model(&entity.DNSBinding{}).Where("status IN ? AND (retry_at IS NULL OR retry_at <= ?) AND (lease_until IS NULL OR lease_until <= ?)", dnsWorkStates, now, now).Pluck("id", &ids).Error
		if err != nil && ctx.Err() == nil {
			zap.L().Warn("读取 DNS 待处理记录失败", zap.Error(err))
		}
		for _, id := range ids {
			if _, exists := running.LoadOrStore(id, true); exists {
				continue
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				err := s.process(ctx, id)
				running.Delete(id)
				if err != nil && ctx.Err() == nil {
					zap.L().Warn("处理 DNS 任务失败", zap.Uint("binding", id), zap.Error(err))
				}
				// 连续改址可能在上一笔请求期间到达。
				if err == nil {
					wakeDNS()
				}
			}()
		}
		select {
		case <-ctx.Done():
			return
		case <-dnsWake:
		case <-ticker.C:
		}
	}
}

func (s *DNSService) process(parent context.Context, id uint) error {
	dnsControls.RLock()
	defer dnsControls.RUnlock()
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	db := repository.DB
	now := time.Now()
	until := now.Add(time.Minute)
	claim := db.Model(&entity.DNSBinding{}).Where("id = ? AND status IN ? AND (lease_until IS NULL OR lease_until <= ?) AND (retry_at IS NULL OR retry_at <= ?)", id, dnsWorkStates, now, now).Update("lease_until", until)
	if claim.Error != nil || claim.RowsAffected == 0 {
		return claim.Error
	}
	defer db.Model(&entity.DNSBinding{}).Where("id = ? AND lease_until = ?", id, until).Update("lease_until", nil)
	var b entity.DNSBinding
	if err := db.First(&b, id).Error; err != nil {
		return err
	}
	if b.Status != entity.DNSPending && b.Status != entity.DNSChecking {
		return nil
	}
	cfg, err := repository.DNSConfig(db)
	if err != nil {
		return err
	}
	var node entity.Node
	if err := db.First(&node, b.NodeID).Error; err != nil {
		return err
	}
	if !cfg.Enabled || !node.Active {
		return finishDNS(b, entity.DNSReview, "同步已暂停", nil)
	}
	if cfg.ZoneID != b.ZoneID {
		return finishDNS(b, entity.DNSRebind, "Zone 已变更，请重新绑定", nil)
	}
	record, err := s.lookup(ctx, cfg, b.Domain, b.Type)
	if err != nil {
		return failDNS(b, err)
	}
	if record.ID != b.RecordID {
		return finishDNS(b, entity.DNSRebind, "记录已重建，请重新绑定", nil)
	}
	remote := record.Content
	if remote == b.DesiredIP {
		return finishDNS(b, entity.DNSSynced, "Cloudflare 已确认记录", map[string]any{"confirmed_ip": remote, "flight_ip": "", "flight_version": 0})
	}
	if remote != b.ConfirmedIP && remote != b.FlightIP {
		return finishDNS(b, entity.DNSRebind, "远端已改为 "+remote+"，请重新预览并绑定", nil)
	}
	if b.Status == entity.DNSChecking {
		return finishDNS(b, entity.DNSReview, "已核对远端记录，可手动同步", map[string]any{"confirmed_ip": remote, "flight_ip": "", "flight_version": 0})
	}

	// 读取远端后再用版本条件抢占本次写入，过时的期望不能派发。
	res := db.Model(&entity.DNSBinding{}).Where("id = ? AND version = ? AND status = ?", id, b.Version, entity.DNSPending).
		Updates(map[string]any{"confirmed_ip": remote, "flight_ip": b.DesiredIP, "flight_version": b.Version})
	if res.Error != nil || res.RowsAffected == 0 {
		return res.Error
	}
	body, _ := json.Marshal(map[string]string{"content": b.DesiredIP})
	var receipt dnsRecord
	err = s.request(ctx, cfg, "PATCH", "/"+url.PathEscape(record.ID), body, &receipt)
	if err != nil {
		return failDNS(b, err)
	}
	actual, kind := entity.DNSAddress(receipt.Content)
	if actual != b.DesiredIP || kind != b.Type || receipt.ID != record.ID || entity.DNSName(receipt.Name) != b.Domain {
		return failDNS(b, &DNSUpstreamError{"更新回执不完整，将重新读取远端记录", true, 0})
	}
	// 按版本关闭任务，迟到的成功回执只更新已确认地址。
	return finishDNS(b, entity.DNSSynced, "Cloudflare 已确认记录", map[string]any{"confirmed_ip": actual, "flight_ip": "", "flight_version": 0})
}

func finishDNS(b entity.DNSBinding, status, message string, extra map[string]any) error {
	updates := map[string]any{"status": status, "message": message, "retry_at": nil}
	for k, v := range extra {
		updates[k] = v
	}
	return repository.DB.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&entity.DNSBinding{}).Where("id = ? AND version = ? AND status = ?", b.ID, b.Version, b.Status).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			if status != entity.DNSSynced {
				return nil
			}
			// 改址期间收到上一版本的回执，保留新任务的状态与操作者。
			ack := tx.Model(&entity.DNSBinding{}).Where("id = ? AND flight_version = ? AND flight_ip = ?", b.ID, b.Version, b.DesiredIP).
				Updates(map[string]any{"confirmed_ip": b.DesiredIP, "flight_ip": "", "flight_version": 0})
			if ack.Error != nil || ack.RowsAffected == 0 {
				return ack.Error
			}
		}
		return dnsAudit(tx, b.NodeID, b.Actor, status == entity.DNSSynced, fmt.Sprintf("%s %s → %s，%s", b.Domain, b.ConfirmedIP, b.DesiredIP, message))
	})
}

func failDNS(b entity.DNSBinding, err error) error {
	if errors.Is(err, errRecordConflict) {
		return finishDNS(b, entity.DNSRebind, err.Error(), nil)
	}
	var f *DNSUpstreamError
	if errors.As(err, &f) && f.transient && b.Retries < 2 {
		delay := max(f.retryAfter, time.Duration(1<<b.Retries)*5*time.Second)
		return finishDNS(b, b.Status, f.message+"，等待重试", map[string]any{"retry_at": time.Now().Add(delay), "retries": b.Retries + 1})
	}
	return finishDNS(b, entity.DNSFailed, err.Error(), nil)
}
