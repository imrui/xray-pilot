package service

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/imrui/xray-pilot/config"
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
}

func NewSyncService() *SyncService {
	return &SyncService{
		nodeRepo:    repository.NewNodeRepository(),
		userRepo:    repository.NewUserRepository(),
		logRepo:     repository.NewLogRepository(),
		profileRepo: repository.NewInboundProfileRepository(),
	}
}

// SyncResult 单节点同步结果
type SyncResult struct {
	NodeID  uint
	Name    string
	Success bool
	Error   string
}

// SyncNode 同步单个节点：生成多协议配置 → 推送 → 重载 xray → 更新状态
func (s *SyncService) SyncNode(nodeID uint) *SyncResult {
	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return &SyncResult{NodeID: nodeID, Error: "节点不存在"}
	}

	result := &SyncResult{NodeID: nodeID, Name: node.Name}

	configContent, err := s.buildConfig(node)
	if err != nil {
		result.Error = fmt.Sprintf("生成配置失败: %v", err)
		s.failNode(node, result.Error, 0)
		return result
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

// SyncDrifted 仅同步状态为 drifted 或 failed 的节点
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

// CheckDrift 拉取远端配置并与本地 ConfigHash 比对
func (s *SyncService) CheckDrift(nodeID uint) (drifted bool, err error) {
	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return false, fmt.Errorf("节点不存在")
	}
	if node.ConfigHash == "" {
		return false, nil
	}

	params := s.sshParams(node)
	remoteContent, err := xray.ReadRemoteConfig(params)
	if err != nil {
		return false, fmt.Errorf("读取远端配置失败: %w", err)
	}

	remoteHash := xray.ConfigHash(remoteContent)
	if remoteHash != node.ConfigHash {
		_ = s.nodeRepo.UpdateSyncStatus(nodeID, entity.SyncStatusDrifted, "")
		s.logRepo.Record(
			"drift_check",
			fmt.Sprintf("node:%s(%d)", node.Name, nodeID),
			false,
			fmt.Sprintf("配置漂移: local=%s remote=%s", node.ConfigHash[:8], remoteHash[:8]),
			0,
		)
		return true, nil
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

func (s *SyncService) buildConfig(node *entity.Node) (string, error) {
	// 查询节点关联的所有激活协议密钥
	profileKeys, err := s.profileRepo.FindActiveKeysForNode(node.ID)
	if err != nil {
		return "", fmt.Errorf("查询节点协议配置失败: %w", err)
	}

	// 查询节点对应分组内的激活用户
	users, err := s.userRepo.FindActiveUsersByNodeID(node.ID)
	if err != nil {
		return "", fmt.Errorf("查询节点用户失败: %w", err)
	}

	return xray.GenerateConfig(node, profileKeys, users)
}

func (s *SyncService) sshParams(node *entity.Node) xray.SSHParams {
	sshPort := node.SSHPort
	if sshPort == 0 {
		sshPort = config.Global.SSH.DefaultPort
		if sshPort == 0 {
			sshPort = 22
		}
	}
	sshUser := node.SSHUser
	if sshUser == "" {
		sshUser = config.Global.SSH.DefaultUser
		if sshUser == "" {
			sshUser = "root"
		}
	}
	keyPath := node.SSHKeyPath
	if keyPath == "" {
		keyPath = config.Global.SSH.DefaultKeyPath
	}
	return xray.SSHParams{
		Host:    node.IP,
		Port:    sshPort,
		User:    sshUser,
		KeyPath: keyPath,
	}
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
