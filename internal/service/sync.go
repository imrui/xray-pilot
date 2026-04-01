package service

import (
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
)

// SyncService 节点配置同步与漂移检测服务
type SyncService struct {
	nodeRepo    *repository.NodeRepository
	userRepo    *repository.UserRepository
	logRepo     *repository.LogRepository
	profileRepo *repository.InboundProfileRepository
	settingSvc  *SettingService
}

var errNodeNoProtocols = errors.New("节点未绑定协议，无法生成可用配置")

func NewSyncService() *SyncService {
	return &SyncService{
		nodeRepo:    repository.NewNodeRepository(),
		userRepo:    repository.NewUserRepository(),
		logRepo:     repository.NewLogRepository(),
		profileRepo: repository.NewInboundProfileRepository(),
		settingSvc:  NewSettingService(),
	}
}

// SyncResult 单节点同步结果
type SyncResult struct {
	NodeID       uint
	Name         string
	Success      bool
	Error        string
	InboundCount int      // 成功生成的 inbound 数量
	Warnings     []string // inbound 生成警告（某个协议失败但整体继续）
}

// SyncNode 同步单个节点：生成多协议配置 → 推送 → 重载 xray → 更新状态
func (s *SyncService) SyncNode(nodeID uint) *SyncResult {
	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return &SyncResult{NodeID: nodeID, Error: "节点不存在"}
	}

	result := &SyncResult{NodeID: nodeID, Name: node.Name}

	configContent, warnings, err := s.buildConfig(node)
	if err != nil {
		result.Error = fmt.Sprintf("生成配置失败: %v", err)
		s.failNode(node, result.Error, 0)
		return result
	}
	result.Warnings = warnings
	if len(warnings) > 0 {
		zap.L().Warn("部分协议 inbound 生成失败", zap.String("node", node.Name), zap.Strings("warnings", warnings))
	}

	params := s.sshParams(node)
	syncResult := xray.SyncWithFallback(params, configContent)

	if !syncResult.Success {
		result.Error = syncResult.Error
		s.failNode(node, result.Error, syncResult.ElapsedMs)
		return result
	}

	newHash := xray.ConfigHash(configContent)

	if err := s.nodeRepo.UpdateLastSync(nodeID, entity.SyncStatusSynced, newHash); err != nil {
		zap.L().Warn("更新同步状态失败", zap.Uint("nodeID", nodeID), zap.Error(err))
	}

	// 同步时顺带更新 XrayVersion
	if syncResult.XrayVersion != "" {
		_ = s.nodeRepo.UpdateXrayStatus(nodeID, true, syncResult.XrayVersion)
	}

	s.logRepo.Record("sync", fmt.Sprintf("node:%s(%d)", node.Name, nodeID), true, "同步成功", syncResult.ElapsedMs)

	result.Success = true
	return result
}

// SyncAll 同步所有激活节点
func (s *SyncService) SyncAll() []SyncResult {
	nodes, err := s.nodeRepo.FindAll()
	if err != nil {
		return nil
	}
	results := make([]SyncResult, 0, len(nodes))
	for _, node := range nodes {
		node := node
		results = append(results, *s.SyncNode(node.ID))
	}
	return results
}

// SyncDrifted 同步状态为 drifted、failed 或 pending 的节点
func (s *SyncService) SyncDrifted() []SyncResult {
	nodes, err := s.nodeRepo.GetDriftedNodes()
	if err != nil {
		return nil
	}
	results := make([]SyncResult, 0, len(nodes))
	for _, node := range nodes {
		node := node
		results = append(results, *s.SyncNode(node.ID))
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
			s.logRepo.Record(
				"drift_check",
				fmt.Sprintf("node:%s(%d)", node.Name, nodeID),
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
		s.logRepo.Record(
			"drift_check",
			fmt.Sprintf("node:%s(%d)", node.Name, nodeID),
			false,
			fmt.Sprintf("远端配置读取异常: %v", err),
			0,
		)
		return false, fmt.Errorf("读取远端配置失败: %w", err)
	}
	remoteContent = strings.TrimSpace(remoteContent)
	if remoteContent == "" {
		s.logRepo.Record(
			"drift_check",
			fmt.Sprintf("node:%s(%d)", node.Name, nodeID),
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
			s.logRepo.Record(
				"drift_check",
				fmt.Sprintf("node:%s(%d)", node.Name, nodeID),
				false,
				fmt.Sprintf("远端配置漂移: expected=%s remote=%s", expectedHash[:8], remoteHash[:8]),
				0,
			)
		}
		return true, nil
	}
	if node.SyncStatus != entity.SyncStatusSynced || node.ConfigHash != expectedHash {
		_ = s.nodeRepo.UpdateLastSync(nodeID, entity.SyncStatusSynced, expectedHash)
	}
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
	// 查询节点关联的所有激活协议密钥（LEFT JOIN 语义，无节点密钥则 fallback 到协议默认值）
	profileKeys, err := s.profileRepo.FindActiveKeysForNode(node.ID)
	if err != nil {
		return "", nil, fmt.Errorf("查询节点协议配置失败: %w", err)
	}
	if len(profileKeys) == 0 {
		return "", nil, errNodeNoProtocols
	}

	// 查询节点对应分组内的激活用户
	users, err := s.userRepo.FindActiveUsersByNodeID(node.ID)
	if err != nil {
		return "", nil, fmt.Errorf("查询节点用户失败: %w", err)
	}

	logCfg := xray.LogConfig{
		Access: s.settingSvc.Get(KeyXrayLogAccess),
		Error:  s.settingSvc.Get(KeyXrayLogError),
		Level:  s.settingSvc.Get(KeyXrayLogLevel),
	}
	return xray.GenerateConfig(node, profileKeys, users, logCfg)
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
	// 脱敏：将生成配置中的 privateKey 替换为占位符（避免通过 API 暴露）
	masked := maskXrayConfig(content)
	return masked, warnings, nil
}

func (s *SyncService) sshParams(node *entity.Node) xray.SSHParams {
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

func (s *SyncService) failNode(node *entity.Node, errMsg string, elapsedMs int64) {
	_ = s.nodeRepo.UpdateSyncStatus(node.ID, entity.SyncStatusFailed, "")
	zap.L().Error("节点同步失败",
		zap.String("node", node.Name),
		zap.Uint("id", node.ID),
		zap.String("error", errMsg),
	)
	s.logRepo.Record(
		"sync",
		fmt.Sprintf("node:%s(%d)", node.Name, node.ID),
		false,
		errMsg,
		elapsedMs,
	)
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
