package service

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

type OutboundProbeResult struct {
	OK        bool      `json:"ok"`
	ElapsedMs int64     `json:"elapsed_ms"`
	CheckedAt time.Time `json:"checked_at"`
	Message   string    `json:"message"`
}

var (
	ErrOutboundProbeBusy        = errors.New("该节点正在测试出站，请稍后再试")
	ErrOutboundNotConfigured    = errors.New("请选择该节点已保存路由中的 VLESS 自定义出站")
	ErrOutboundRouteUnavailable = errors.New("无法解析节点已保存的路由，请先检查路由绑定")
	// 手动诊断使用独立临时客户端，无需占用生产配置同步与过期撤权的租约。
	outboundProbes sync.Map
)

var probeRemoteOutbound = func(ctx context.Context, p xray.SSHParams, tag string) (time.Duration, error) {
	c, err := xssh.ConnectContext(ctx, xssh.Config{
		Host: p.Host, Port: p.Port, User: p.User, KeyPath: p.KeyPath, KnownHostsPath: p.KnownHostsPath,
	})
	if err != nil {
		return 0, errors.New("无法通过 SSH 连接节点，请检查节点连接参数")
	}
	defer c.Close()
	// 此接口只返回预定义的诊断错误，不公开远端命令输出或配置内容。
	return c.TestVLESSOutbound(p.ConfigPath, tag, p.ProbeURL)
}

func (s *SyncService) TestOutbound(ctx context.Context, nodeID uint, tag string) (*OutboundProbeResult, error) {
	if len(tag) == 0 || len(tag) > 63 {
		return nil, ErrOutboundNotConfigured
	}
	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return nil, err
	}
	effective, err := resolveRouting(repository.DB.WithContext(ctx), nodeID)
	if err != nil {
		return nil, ErrOutboundRouteUnavailable
	}
	if effective.Policy == nil || !slices.ContainsFunc(effective.Policy.CustomOutbounds, func(o xray.CustomOutbound) bool {
		return o.Tag == tag && o.Protocol == "vless"
	}) {
		return nil, ErrOutboundNotConfigured
	}
	if _, busy := outboundProbes.LoadOrStore(nodeID, struct{}{}); busy {
		return nil, ErrOutboundProbeBusy
	}
	defer outboundProbes.Delete(nodeID)

	ctx, cancel := context.WithTimeout(ctx, 55*time.Second)
	defer cancel()
	elapsed, err := probeRemoteOutbound(ctx, s.sshParamsForPath(node, effective.Binding.ConfigPath), tag)
	result := &OutboundProbeResult{CheckedAt: time.Now().UTC(), ElapsedMs: elapsed.Milliseconds()}
	switch {
	case ctx.Err() != nil:
		result.Message = "测试已超时或取消"
	case err != nil:
		result.Message = err.Error()
	default:
		result.OK = true
		result.Message = "已通过此出站访问 HTTPS 探测地址"
	}
	return result, nil
}
