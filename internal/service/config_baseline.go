package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
	"gorm.io/gorm"
)

var ErrBaselineChanged = errors.New("远端文件在核对后已发生变化，请重新核对")

type BaselineInspection struct {
	ConfigPath           string `json:"config_path"`
	ObservedRawSHA256    string `json:"observed_raw_sha256"`
	ObservedHash         string `json:"observed_hash"`
	RecordedBaselineHash string `json:"recorded_baseline_hash"`
	DesiredHash          string `json:"desired_hash"`
	BaselineMatched      bool   `json:"baseline_matched"`
	RemoteValidJSON      bool   `json:"remote_valid_json"`
	RemoteBytes          int    `json:"remote_bytes"`
	RemoteRedacted       string `json:"remote_redacted"`
	DesiredRedacted      string `json:"desired_redacted"`
	DesiredError         string `json:"desired_error,omitempty"`
}

func recordedBaselineHash(node *entity.Node, state *entity.NodeConfigState) string {
	if state.DiskHash != "" {
		return state.DiskHash
	}
	return node.ConfigHash
}

func rawSHA256(raw string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
}

func (s *SyncService) InspectBaseline(nodeID uint) (*BaselineInspection, error) {
	if hasActiveConfigLease(nodeID) {
		return nil, ErrNodeSyncBusy
	}
	defer lockNode(nodeID)()
	if hasActiveConfigLease(nodeID) {
		return nil, ErrNodeSyncBusy
	}
	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return nil, err
	}
	var state entity.NodeConfigState
	if err := repository.DB.Where("node_id = ?", nodeID).Find(&state).Error; err != nil {
		return nil, err
	}
	p := s.sshParams(node)
	raw, err := xray.ReadRemoteConfig(p)
	if err != nil {
		return nil, err
	}
	out := &BaselineInspection{
		ConfigPath: p.ConfigPath, ObservedRawSHA256: rawSHA256(raw), ObservedHash: xray.ConfigHash(raw),
		RecordedBaselineHash: recordedBaselineHash(node, &state), RemoteBytes: len(raw),
		RemoteValidJSON: json.Valid([]byte(raw)), RemoteRedacted: "<invalid json>",
	}
	if out.RemoteValidJSON {
		out.RemoteRedacted = redactConfig(raw)
	}
	desired, _, err := s.buildConfig(node)
	if err == nil {
		out.DesiredHash, out.DesiredRedacted = xray.ConfigHash(desired), redactConfig(desired)
	} else {
		out.DesiredError = "无法生成期望配置，请检查节点绑定的协议与路由。"
	}
	out.BaselineMatched = xssh.ConfigBaselineMatched(raw, desired, xssh.TransactionOptions{
		BaselineHash: out.RecordedBaselineHash, AllowInitial: state.InitialBaseline, InitialBaselineHash: state.InitialBaselineHash,
	})
	return out, nil
}

type AdoptBaselineRequest struct {
	ObservedRawSHA256 string `json:"observed_raw_sha256"`
	Key               string `json:"request_key"`
}

func (s *SyncService) AdoptBaseline(nodeID uint, req AdoptBaselineRequest, actor string) ([]entity.ConfigDeployment, error) {
	digest, err := hex.DecodeString(req.ObservedRawSHA256)
	if err != nil || len(digest) != sha256.Size {
		return nil, errors.New("请提供核对时的 64 位十六进制 SHA-256")
	}
	if n := utf8.RuneCountInString(req.Key); n < 8 || n > 100 || strings.TrimSpace(req.Key) == "" {
		return nil, errors.New("请提供 8–100 字符幂等键")
	}
	defer lockNode(nodeID)()
	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return nil, err
	}
	state, release, err := repository.AcquireConfigLease(nodeID)
	if errors.Is(err, repository.ErrConfigLeased) {
		return nil, ErrNodeSyncBusy
	}
	if err != nil {
		return nil, err
	}
	defer release()
	node, err = s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return nil, err
	}
	// 普通入队键以节点 ID 结尾，使用独立后缀以免与已有任务混淆。
	key := fmt.Sprintf("%d:%s:adopt-baseline", nodeID, req.Key)
	var job entity.ConfigDeployment
	if err := repository.DB.Where("request_key = ?", key).Take(&job).Error; err == nil {
		return []entity.ConfigDeployment{job}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	p := s.sshParams(node)
	raw, err := xray.ReadRemoteConfig(p)
	if err != nil {
		return nil, err
	}
	observed := rawSHA256(raw)
	if !strings.EqualFold(observed, req.ObservedRawSHA256) {
		return nil, ErrBaselineChanged
	}
	desired, _, err := s.buildConfig(node)
	if err != nil {
		return nil, fmt.Errorf("生成配置失败: %w", err)
	}
	if xssh.ConfigBaselineMatched(raw, desired, xssh.TransactionOptions{
		BaselineHash: recordedBaselineHash(node, state), AllowInitial: state.InitialBaseline, InitialBaselineHash: state.InitialBaselineHash,
	}) {
		return nil, errors.New("远端配置已通过基线校验，可直接重试同步")
	}
	blank := strings.TrimSpace(raw) == ""
	diskHash, initialHash := xray.ConfigHash(raw), ""
	if blank {
		diskHash, initialHash = "", observed
	}
	job = entity.ConfigDeployment{NodeID: nodeID, RequestKey: key, Actor: actor, Status: "pending", DesiredHash: xray.ConfigHash(desired)}
	err = repository.DB.Transaction(func(tx *gorm.DB) error {
		// 认领、释放租约、审计与入队一并提交，队列失败时恢复原基线。
		if err := leaseUpdate(tx.Where("change_revision = ?", state.ChangeRevision), state, map[string]any{
			"disk_hash": diskHash, "initial_baseline": blank, "initial_baseline_hash": initialHash,
			"runtime_hash": "", "runtime_authorization_hash": "", "verified_at": nil,
			"recovery_attempts": 0, "next_retry_at": nil, "last_error": "", "lease_owner": "", "lease_until": nil,
		}); err != nil {
			return err
		}
		if blank {
			if err := tx.Model(node).Update("config_hash", "").Error; err != nil {
				return err
			}
		}
		if err := s.logRepo.WithTx(tx).RecordWithActor("adopt_baseline", fmt.Sprintf("node:%s(%d)", node.Name, node.ID), actor, true,
			fmt.Sprintf("接管远端基线 %s，SHA-256 %s", p.ConfigPath, observed[:12]), 0); err != nil {
			return err
		}
		return enqueueDeployment(tx, &job)
	})
	if err != nil {
		return nil, err
	}
	return []entity.ConfigDeployment{job}, nil
}
