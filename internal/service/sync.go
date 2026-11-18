package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

// SyncService 节点配置同步与漂移检测服务
type SyncService struct {
	nodeRepo    *repository.NodeRepository
	userRepo    *repository.UserRepository
	logRepo     *repository.LogRepository
	profileRepo *repository.InboundProfileRepository
	settingSvc  *SettingService
	alertSvc    *AlertService
}

var errNodeNoProtocols = errors.New("节点未绑定协议，无法生成可用配置")

var pushConfig = xray.SyncWithFallback

func NewSyncService() *SyncService {
	return &SyncService{
		nodeRepo:    repository.NewNodeRepository(),
		userRepo:    repository.NewUserRepository(),
		logRepo:     repository.NewLogRepository(),
		profileRepo: repository.NewInboundProfileRepository(),
		settingSvc:  NewSettingService(),
		alertSvc:    NewAlertService(),
	}
}

// SyncResult 单节点同步结果
type SyncResult struct {
	NodeID       uint
	Name         string
	Success      bool
	Error        string
	InboundCount int // 成功生成的 inbound 数量
	Busy         bool
	Warnings     []string // inbound 生成警告（某个协议失败但整体继续）
}

// actorSchedulerDriftCheck 漂移检测只由调度器触发
const actorSchedulerDriftCheck = "system:scheduler:drift_check"

// SyncNode 同步单个节点：生成多协议配置 → 推送 → 重载 xray → 更新状态。actor 为触发方（见 entity.SyncLog）
func (s *SyncService) SyncNode(nodeID uint, actor string) *SyncResult {
	return s.syncExpected(nodeID, actor, "")
}
func (s *SyncService) syncExpected(nodeID uint, actor, expected string) *SyncResult {
	defer lockNode(nodeID)()

	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return &SyncResult{NodeID: nodeID, Error: "节点不存在"}
	}

	result := &SyncResult{NodeID: nodeID, Name: node.Name}
	state, release, err := repository.AcquireConfigLease(nodeID)
	if err != nil {
		result.Error = err.Error()
		result.Busy = errors.Is(err, repository.ErrConfigLeased)
		return result
	}
	defer release()
	// 获锁后重新判断，避免等待热更新的过期恢复任务再次重启。
	if strings.HasPrefix(actor, "system:") && !state.RecoveryPending && state.VerifiedAt != nil && state.RuntimeHash != "" && state.RuntimeHash == state.DiskHash {
		raw, _, e := s.buildConfig(node)
		if e == nil && xray.ConfigHash(raw) == state.RuntimeHash {
			result.Success = true
			return result
		}
	}

	configContent, warnings, err := s.buildConfig(node)
	if err != nil {
		result.Error = fmt.Sprintf("生成配置失败: %v", err)
		s.failNode(node, actor, result.Error, 0)
		return result
	}
	result.Warnings = warnings
	snapshot, err := NewRoutingService().Effective(node.ID)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if expected != "" && xray.ConfigHash(configContent) != expected {
		result.Error = "期望配置已更新，请重新下发"
		return result
	}
	if len(warnings) > 0 {
		zap.L().Warn("部分协议 inbound 生成失败", zap.String("node", node.Name), zap.Strings("warnings", warnings))
	}

	params := s.sshParams(node)
	params.RequiresIPv6 = snapshot.Policy != nil && snapshot.Policy.RequiresIPv6WithOptions(xray.RenderOptions{DefaultProbeURL: params.ProbeURL})
	if state.VerifiedAt != nil && state.RuntimeHash == xray.ConfigHash(configContent) && state.DiskHash == state.RuntimeHash {
		params.ConfirmedProcess = state.ProcessID
	}
	if err := rememberDesired(state, xray.ConfigHash(configContent)); err != nil {
		result.Error = "无法保存期望版本"
		return result
	}
	params.BaselineHash = state.DiskHash
	if params.BaselineHash == "" {
		params.BaselineHash = node.ConfigHash
	}
	params.AllowInitial = state.InitialBaseline
	params.InitialBaselineHash = state.InitialBaselineHash
	params.OnBaseline = func(hash string) error {
		if !s.stillCurrent(state, xray.ConfigHash(configContent)) {
			return fmt.Errorf("期望配置已更新，请重新下发")
		}
		if err := leaseUpdate(repository.DB, state, map[string]any{"initial_baseline": false, "initial_baseline_hash": "", "disk_hash": hash}); err != nil {
			return err
		}
		state.InitialBaseline = false
		state.InitialBaselineHash = ""
		state.DiskHash = hash
		return nil
	}
	params.RollbackContent, _, err = s.buildConfigPolicy(node, state.LastHealthyPolicy)
	if err != nil {
		result.Error = "恢复配置生成失败：" + err.Error()
		return result
	}
	params.OnPhase = deploymentPhaseRecorder(state)
	params.BeforeApply = func() error {
		if !s.stillCurrent(state, xray.ConfigHash(configContent)) {
			return fmt.Errorf("期望配置已更新，请重新下发")
		}
		return nil
	}
	syncResult := pushConfig(params, configContent)

	if !syncResult.Success {
		result.Error = syncResult.Error
		if strings.Contains(result.Error, "AUTHORIZATION_STOPPED") {
			if err := s.recoverHealthyPolicy(node.ID, state.LeaseOwner, actor, xray.ConfigHash(configContent)); err != nil {
				result.Error = "ROLLBACK_FAILED：" + err.Error()
			} else {
				result.Error = "ROLLED_BACK：已恢复最近健康策略，并保留当前用户与密钥"
			}
		}
		s.failNode(node, actor, result.Error, syncResult.ElapsedMs)
		if syncResult.ObservedHash != "" {
			_ = leaseUpdate(repository.DB, state, map[string]any{"disk_hash": syncResult.ObservedHash, "disk_revision": state.DesiredRevision, "runtime_hash": "", "runtime_authorization_hash": "", "verified_at": nil})
		}
		if strings.Contains(result.Error, "ROLLED_BACK") {
			if syncResult.RecoveredHash != "" {
				updates := map[string]any{"disk_hash": syncResult.RecoveredHash, "runtime_hash": "", "runtime_authorization_hash": "", "verified_at": nil}
				if syncResult.ProcessID != "" && syncResult.RecoveredAuthorizationHash != "" && syncResult.RecoveredAuthorizationHash == xssh.AuthorizationHash(configContent) && s.stillCurrent(state, xray.ConfigHash(configContent)) {
					updates["runtime_hash"] = syncResult.RecoveredHash
					updates["runtime_authorization_hash"] = syncResult.RecoveredAuthorizationHash
					updates["verified_at"] = time.Now()
					updates["process_id"] = syncResult.ProcessID
					updates["verification"] = "recovery_service"
				}
				_ = leaseUpdate(repository.DB, state, updates)
			}
			_ = s.nodeRepo.UpdateSyncStatus(node.ID, entity.SyncStatusDrifted, "")
		}
		return result
	}

	newHash := xray.ConfigHash(configContent)
	status := entity.SyncStatusDrifted
	if s.stillCurrent(state, newHash) {
		status = entity.SyncStatusSynced
	}
	if err := s.recordApplied(state, configContent, syncResult.ProcessID, snapshot); err != nil {
		result.Error = "节点已应用，但回执未保存，需核对"
		return result
	}
	if err := finishNodeStatus(state, newHash, status == entity.SyncStatusSynced); err != nil {
		result.Error = err.Error()
		return result
	}

	// 同步时顺带更新 XrayVersion
	if syncResult.XrayVersion != "" {
		_ = s.nodeRepo.UpdateXrayStatus(nodeID, true, syncResult.XrayVersion)
	}

	s.logRepo.RecordWithActor("sync", fmt.Sprintf("node:%s(%d)", node.Name, nodeID), actor, true, "同步成功", syncResult.ElapsedMs)
	s.reportSyncAlert(nodeID, true, "")

	if status != entity.SyncStatusSynced {
		result.Error = "期望配置已更新，旧版本回执已记录"
		return result
	}
	result.Success = true
	return result
}

// SyncAll 同步所有激活节点
func (s *SyncService) SyncAll(actor string) []SyncResult {
	nodes, err := s.nodeRepo.FindAll()
	if err != nil {
		return nil
	}
	results := make([]SyncResult, 0, len(nodes))
	for _, node := range nodes {
		node := node
		results = append(results, *s.SyncNode(node.ID, actor))
	}
	return results
}

// SyncDrifted 同步状态为 drifted、failed 或 pending 的节点
func (s *SyncService) SyncDrifted(actor string) []SyncResult {
	nodes, err := s.nodeRepo.GetDriftedNodes()
	if err != nil {
		return nil
	}
	results := make([]SyncResult, 0, len(nodes))
	for _, node := range nodes {
		node := node
		var state entity.NodeConfigState
		if strings.HasPrefix(actor, "system:") && repository.DB.First(&state, "node_id = ?", node.ID).Error == nil && (state.RecoveryAttempts >= 3 || state.NextRetryAt != nil && state.NextRetryAt.After(time.Now())) {
			continue
		}
		results = append(results, *s.SyncNode(node.ID, actor))
	}
	return results
}

// CheckDrift 重新生成期望配置，区分“控制面配置已变化”和“远端配置漂移”：
//   - 若当前期望 hash 与数据库记录的已同步 hash 不一致，说明源数据已变化，节点需要重新同步
//   - 若当前期望 hash 与数据库记录一致，再读取远端文件；只有远端 hash 不一致才判定为远端漂移
func (s *SyncService) CheckDrift(nodeID uint) (drifted bool, err error) {
	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return false, fmt.Errorf("节点不存在")
	}
	// 尚未同步过的节点跳过漂移检测
	if node.SyncStatus == entity.SyncStatusPending {
		return false, nil
	}

	expectedContent, _, err := s.buildConfig(node)
	if err != nil {
		return false, fmt.Errorf("生成期望配置失败: %w", err)
	}

	expectedHash := xray.ConfigHash(expectedContent)
	if expectedHash != node.ConfigHash {
		_ = s.nodeRepo.UpdateSyncStatus(nodeID, entity.SyncStatusDrifted, "")
		if node.SyncStatus != entity.SyncStatusDrifted {
			s.logRepo.RecordWithActor(
				"drift_check",
				fmt.Sprintf("node:%s(%d)", node.Name, nodeID),
				actorSchedulerDriftCheck,
				false,
				fmt.Sprintf("配置源已变化，节点需重新同步: last=%s expected=%s", shortHash(node.ConfigHash), expectedHash[:8]),
				0,
			)
		}
		return true, nil
	}

	params := s.sshParams(node)
	remoteContent, err := xray.ReadRemoteConfig(params)
	if err != nil {
		s.logRepo.RecordWithActor(
			"drift_check",
			fmt.Sprintf("node:%s(%d)", node.Name, nodeID),
			actorSchedulerDriftCheck,
			false,
			fmt.Sprintf("远端配置读取异常: %v", err),
			0,
		)
		return false, fmt.Errorf("读取远端配置失败: %w", err)
	}
	remoteContent = strings.TrimSpace(remoteContent)
	if remoteContent == "" {
		s.logRepo.RecordWithActor(
			"drift_check",
			fmt.Sprintf("node:%s(%d)", node.Name, nodeID),
			actorSchedulerDriftCheck,
			false,
			"远端配置读取异常: 内容为空",
			0,
		)
		return false, fmt.Errorf("远端配置为空")
	}

	remoteHash := xray.ConfigHash(remoteContent)
	zap.L().Debug("漂移检测读取远端配置",
		zap.String("node", node.Name),
		zap.Uint("nodeID", node.ID),
		zap.Int("remote_bytes", len(remoteContent)),
		zap.String("expected_hash", shortHash(expectedHash)),
		zap.String("remote_hash", shortHash(remoteHash)),
	)
	if expectedHash != remoteHash {
		_ = s.nodeRepo.UpdateSyncStatus(nodeID, entity.SyncStatusDrifted, "")
		if node.SyncStatus != entity.SyncStatusDrifted {
			s.logRepo.RecordWithActor(
				"drift_check",
				fmt.Sprintf("node:%s(%d)", node.Name, nodeID),
				actorSchedulerDriftCheck,
				false,
				fmt.Sprintf("远端配置漂移: expected=%s remote=%s", expectedHash[:8], remoteHash[:8]),
				0,
			)
		}
		return true, nil
	}
	// Matching disk bytes do not prove runtime convergence after a failed restart/live apply.
	return false, nil
}

// CheckDriftAll 批量检测所有激活节点的配置漂移
func (s *SyncService) CheckDriftAll() (driftCount int, errs []string) {
	nodes, err := s.nodeRepo.FindAll()
	if err != nil {
		return 0, []string{err.Error()}
	}
	for _, node := range nodes {
		node := node
		if node.ConfigHash == "" {
			continue
		}
		drifted, err := s.CheckDrift(node.ID)
		if err != nil {
			errs = append(errs, fmt.Sprintf("node %d: %v", node.ID, err))
			continue
		}
		if drifted {
			driftCount++
		}
	}
	return driftCount, errs
}

// ---- 内部工具方法 ----

func (s *SyncService) buildConfig(node *entity.Node) (string, []string, error) {
	return s.buildConfigPolicy(node, "desired")
}

func (s *SyncService) buildConfigPolicy(node *entity.Node, policy string) (string, []string, error) {
	// 查询节点关联的所有激活协议密钥（LEFT JOIN 语义，无节点密钥则 fallback 到协议默认值）
	profileKeys, err := s.profileRepo.FindActiveKeysForNode(node.ID)
	if err != nil {
		return "", nil, fmt.Errorf("查询节点协议配置失败: %w", err)
	}
	// Previously managed nodes must be able to revoke their last protocol.
	// Keep the API-only configuration so stale listeners are removed on sync.
	if len(profileKeys) == 0 && node.ConfigHash == "" {
		return "", nil, errNodeNoProtocols
	}

	// 查询节点对应分组内的激活用户
	users, err := s.userRepo.FindActiveUsersByNodeID(node.ID)
	if err != nil {
		return "", nil, fmt.Errorf("查询节点用户失败: %w", err)
	}

	// 日志级别：节点级覆盖优先，空则回退系统设置（便于单节点排障不影响其他节点）
	logLevel := node.LogLevel
	if logLevel == "" {
		logLevel = s.settingSvc.Get(KeyXrayLogLevel)
	}
	logCfg := xray.LogConfig{
		Access: s.settingSvc.Get(KeyXrayLogAccess),
		Error:  s.settingSvc.Get(KeyXrayLogError),
		Level:  logLevel,
	}
	content, warnings, err := xray.GenerateConfig(node, profileKeys, users, logCfg)
	if err != nil {
		return "", warnings, err
	}
	if len(warnings) > 0 {
		return "", warnings, fmt.Errorf("协议配置不完整，拒绝覆盖: %s", strings.Join(warnings, "; "))
	}
	renderOpts := xray.RenderOptions{DefaultProbeURL: s.settingSvc.Get(KeyXrayProbeURL)}
	if policy == "desired" {
		effective, e := NewRoutingService().Effective(node.ID)
		if e != nil {
			return "", nil, e
		}
		if effective.Policy != nil {
			content, err = xray.ApplyRoutingPolicyWithOptions(content, *effective.Policy, renderOpts)
		}
	} else if policy != "" && policy != "legacy" {
		p, e := xray.DecodeRoutingPolicy(policy)
		if e != nil {
			return "", nil, e
		}
		content, err = xray.ApplyRoutingPolicyWithOptions(content, p, renderOpts)
	}
	return content, warnings, err
}

// PreviewConfig 生成节点期望配置（供调试用，private_key 脱敏）
func (s *SyncService) PreviewConfig(nodeID uint) (string, []string, error) {
	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return "", nil, fmt.Errorf("节点不存在")
	}
	content, warnings, err := s.buildConfig(node)
	if err != nil {
		return "", warnings, err
	}
	if eff, effErr := NewRoutingService().Effective(nodeID); effErr == nil && eff != nil && eff.Policy != nil {
		warnings = append(warnings, xray.DiagnoseRenderedDNSSelfLoop(content, *eff.Policy, xray.RenderOptions{})...)
	}
	// 脱敏：将生成配置中的 privateKey 替换为占位符（避免通过 API 暴露）
	masked := redactConfig(content)
	return masked, warnings, nil
}

func (s *SyncService) sshParams(node *entity.Node) xray.SSHParams {
	configPath := xray.RemoteConfigPath
	if b, err := NewRoutingService().Binding("node", node.ID); err == nil && b.ConfigPath != "" {
		configPath = b.ConfigPath
	}
	sshPort := node.SSHPort
	if sshPort == 0 {
		sshPort = s.settingSvc.GetInt(KeySSHDefaultPort)
		if sshPort == 0 {
			sshPort = 22
		}
	}
	sshUser := node.SSHUser
	if sshUser == "" {
		sshUser = s.settingSvc.Get(KeySSHDefaultUser)
		if sshUser == "" {
			sshUser = "root"
		}
	}
	keyPath := node.SSHKeyPath
	if keyPath == "" {
		keyPath = s.settingSvc.Get(KeySSHDefaultKeyPath)
	}
	return xray.SSHParams{
		Host:           node.IP,
		ConfigPath:     configPath,
		ProbeURL:       s.settingSvc.Get(KeyXrayProbeURL),
		Port:           sshPort,
		User:           sshUser,
		KeyPath:        keyPath,
		KnownHostsPath: s.settingSvc.Get(KeySSHKnownHostsPath),
	}
}

// maskXrayConfig 将 Xray config JSON 中的 privateKey 字段值替换为占位符
func maskXrayConfig(configJSON string) string {
	const marker = `"privateKey": "`
	const replacement = "<masked>"
	result := configJSON
	offset := 0
	for {
		idx := strings.Index(result[offset:], marker)
		if idx < 0 {
			break
		}
		absStart := offset + idx + len(marker)
		end := strings.Index(result[absStart:], `"`)
		if end < 0 {
			break
		}
		result = result[:absStart] + replacement + result[absStart+end:]
		// 跳过已替换部分，避免对 "<masked>" 重复匹配
		offset = absStart + len(replacement)
	}
	return result
}

func (s *SyncService) failNode(node *entity.Node, actor string, errMsg string, elapsedMs int64) {
	recordConfigFailure(node.ID, errMsg)
	_ = s.nodeRepo.UpdateSyncStatus(node.ID, entity.SyncStatusFailed, "")
	zap.L().Error("节点同步失败",
		zap.String("node", node.Name),
		zap.Uint("id", node.ID),
		zap.String("error", errMsg),
	)
	s.logRepo.RecordWithActor(
		"sync",
		fmt.Sprintf("node:%s(%d)", node.Name, node.ID),
		actor,
		false,
		errMsg,
		elapsedMs,
	)
	s.reportSyncAlert(node.ID, false, truncateRunes(errMsg, 200))
}

// reportSyncAlert 同步结果上报告警：状态同步推进（保证按同步完成顺序生效，失败后紧接成功不会被乱序覆盖），
// 只把发送放进 goroutine，不阻塞同步返回
func (s *SyncService) reportSyncAlert(nodeID uint, ok bool, detail string) {
	if !s.alertSvc.Enabled() {
		return
	}
	s.alertSvc.Observe(nodeID, entity.AlertKindSyncFailed, ok, detail)
	go s.alertSvc.Flush()
}

// truncateRunes 按字符截断，避免卡片被超长错误撑爆
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func shortHash(value string) string {
	if len(value) >= 8 {
		return value[:8]
	}
	if value == "" {
		return "none"
	}
	return value
}
