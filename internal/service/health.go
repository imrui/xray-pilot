package service

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/pkg/types"
)

// actorSchedulerHealthCheck 健康检测只由调度器触发
const actorSchedulerHealthCheck = "system:scheduler:health_check"

// HealthService 节点健康检测：SSH 端口探测 + TCP 代理端口探测，结果喂给告警状态机（v0.5.1 从 scheduler 下沉）
type HealthService struct {
	nodeRepo    *repository.NodeRepository
	profileRepo *repository.InboundProfileRepository
	logRepo     *repository.LogRepository
	alertSvc    *AlertService
}

func NewHealthService() *HealthService {
	return &HealthService{
		nodeRepo:    repository.NewNodeRepository(),
		profileRepo: repository.NewInboundProfileRepository(),
		logRepo:     repository.NewLogRepository(),
		alertSvc:    NewAlertService(),
	}
}

// nodeProbeResult 单节点探测结果
type nodeProbeResult struct {
	node        entity.Node
	sshPort     int
	sshOK       bool
	latencyMs   int
	ports       []int // 探测的 TCP 代理端口（SSH 不可达时不探测，为空）
	failedPorts []int
}

// HealthCheckSummary 一轮检测的汇总
type HealthCheckSummary struct {
	Total, OK, Fail int
}

// RunOnce 检测全部启用节点，写回健康字段并推进告警
func (s *HealthService) RunOnce() (*HealthCheckSummary, error) {
	nodes, err := s.nodeRepo.FindAll()
	if err != nil {
		return nil, err
	}

	results := make([]nodeProbeResult, 0, len(nodes))
	summary := &HealthCheckSummary{Total: len(nodes)}
	for _, node := range nodes {
		r := s.probeNode(node)
		results = append(results, r)
		if r.sshOK {
			summary.OK++
		} else {
			summary.Fail++
		}
		s.persist(r)
	}

	s.reportAlerts(results)
	return summary, nil
}

// probeNode 先探 SSH 端口；可达再探该节点全部 TCP 类代理端口
func (s *HealthService) probeNode(node entity.Node) nodeProbeResult {
	r := nodeProbeResult{node: node, sshPort: node.SSHPort}
	if r.sshPort == 0 {
		r.sshPort = 22
	}
	r.sshOK, r.latencyMs = TCPProbe(node.IP, r.sshPort)
	if !r.sshOK {
		return r
	}

	keys, err := s.profileRepo.FindActiveKeysForNode(node.ID)
	if err != nil {
		zap.L().Warn("健康检测：查询节点协议失败", zap.Uint("nodeID", node.ID), zap.Error(err))
		return r
	}
	r.ports = nodeTCPProxyPorts(keys)
	for _, port := range r.ports {
		if ok, _ := TCPProbe(node.IP, port); !ok {
			r.failedPorts = append(r.failedPorts, port)
		}
	}
	return r
}

// persist 写回 LastCheck / XrayActive，并在节点由健康转不健康时记操作日志
func (s *HealthService) persist(r nodeProbeResult) {
	if err := s.nodeRepo.UpdateLastCheck(r.node.ID, r.sshOK, r.latencyMs); err != nil {
		zap.L().Warn("健康检测：更新状态失败", zap.Uint("nodeID", r.node.ID), zap.Error(err))
	}
	// 仅在真正探测过代理端口时回写 XrayActive；SSH 不可达时无法判断，保留原值
	if len(r.ports) > 0 {
		if err := s.nodeRepo.UpdateXrayActive(r.node.ID, len(r.failedPorts) == 0); err != nil {
			zap.L().Warn("健康检测：更新 Xray 状态失败", zap.Uint("nodeID", r.node.ID), zap.Error(err))
		}
	}
	if !r.sshOK && r.node.LastCheckOK {
		s.logRepo.RecordWithActor(
			"health_check",
			fmt.Sprintf("%s(%s)", r.node.Name, r.node.IP),
			actorSchedulerHealthCheck,
			false,
			"节点不可达（TCP 连接失败）",
			int64(r.latencyMs),
		)
	}
}

// reportAlerts 把一轮探测结果喂给告警状态机，轮末合并发送
func (s *HealthService) reportAlerts(results []nodeProbeResult) {
	if !s.alertSvc.Enabled() {
		return
	}

	activeIDs := make([]uint, 0, len(results))
	for _, r := range results {
		activeIDs = append(activeIDs, r.node.ID)
	}
	s.alertSvc.PruneInactive(activeIDs)

	// 全部节点同时 SSH 不可达：大概率是面板出网 / 跨境链路故障，
	// 只推进面板级状态，不推进各节点状态，避免一次链路故障刷屏
	allDown := allSSHDown(results)
	detail := ""
	if allDown {
		detail = fmt.Sprintf("全部 %d 个节点 SSH 均不可达，疑似面板出网或跨境链路故障", len(results))
	}
	s.alertSvc.Observe(0, entity.AlertKindPanelUnreachable, !allDown, detail)

	if !allDown {
		for _, r := range results {
			s.alertSvc.Observe(r.node.ID, entity.AlertKindNodeUnreachable, r.sshOK,
				fmt.Sprintf("SSH 端口 %d 连接失败", r.sshPort))
			// SSH 不可达那一轮不推进 xray_down，避免一次故障双告警
			if r.sshOK && len(r.ports) > 0 {
				s.alertSvc.Observe(r.node.ID, entity.AlertKindXrayDown, len(r.failedPorts) == 0,
					"TCP 端口 "+joinPorts(r.failedPorts)+" 不通")
			}
		}
	}

	s.alertSvc.Flush()
}

// allSSHDown 至少 2 个节点且全部 SSH 不可达
func allSSHDown(results []nodeProbeResult) bool {
	if len(results) < 2 {
		return false
	}
	for _, r := range results {
		if r.sshOK {
			return false
		}
	}
	return true
}

// nodeTCPProxyPorts 节点上需要探测的 TCP 代理端口：有效端口走 effectiveKeyPort，
// UDP 协议（Hysteria2）无法用 TCP 握手判断，跳过；去重后升序
func nodeTCPProxyPorts(keys []entity.NodeProfileKey) []int {
	seen := map[int]bool{}
	ports := make([]int, 0, len(keys))
	for i := range keys {
		key := &keys[i]
		if key.Profile == nil || types.ProtocolTransport(key.Profile.Protocol) != types.TransportTCP {
			continue
		}
		port := effectiveKeyPort(key.Profile, key)
		if port <= 0 || seen[port] {
			continue
		}
		seen[port] = true
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports
}

func joinPorts(ports []int) string {
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, strconv.Itoa(p))
	}
	return strings.Join(parts, "、")
}
