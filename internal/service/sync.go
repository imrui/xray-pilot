package service

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

const xrayConfigPath = "/usr/local/etc/xray/config.json"

// SyncService 节点配置同步与漂移检测服务
type SyncService struct {
	nodeRepo      *repository.NodeRepository
	userRepo      *repository.UserRepository
	logRepo       *repository.LogRepository
	xrayCfgSvc    *XrayConfigService
}

func NewSyncService() *SyncService {
	return &SyncService{
		nodeRepo:   repository.NewNodeRepository(),
		userRepo:   repository.NewUserRepository(),
		logRepo:    repository.NewLogRepository(),
		xrayCfgSvc: NewXrayConfigService(),
	}
}

// SyncResult 单节点同步结果
type SyncResult struct {
	NodeID  uint
	Name    string
	Success bool
	Error   string
}

// SyncNode 同步单个节点：生成配置 → 上传 → 重载 xray → 更新状态
func (s *SyncService) SyncNode(nodeID uint) *SyncResult {
	start := time.Now()
	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return &SyncResult{NodeID: nodeID, Error: "节点不存在"}
	}

	result := &SyncResult{NodeID: nodeID, Name: node.Name}

	configContent, err := s.buildConfig(node)
	if err != nil {
		result.Error = fmt.Sprintf("生成配置失败: %v", err)
		s.failNode(node, result.Error, time.Since(start).Milliseconds())
		return result
	}

	if err := s.pushConfig(node, configContent); err != nil {
		result.Error = fmt.Sprintf("推送配置失败: %v", err)
		s.failNode(node, result.Error, time.Since(start).Milliseconds())
		return result
	}

	newHash := ConfigHash(configContent)
	elapsed := time.Since(start).Milliseconds()

	if err := s.nodeRepo.UpdateLastSync(nodeID, entity.SyncStatusSynced, newHash); err != nil {
		zap.L().Warn("更新同步状态失败", zap.Uint("nodeID", nodeID), zap.Error(err))
	}
	s.logRepo.Record("sync", fmt.Sprintf("node:%s(%d)", node.Name, nodeID), true, "同步成功", elapsed)

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
// 若哈希不一致，将节点置为 drifted
func (s *SyncService) CheckDrift(nodeID uint) (drifted bool, err error) {
	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return false, fmt.Errorf("节点不存在")
	}
	if node.ConfigHash == "" {
		// 从未同步过，视为 pending，跳过漂移检测
		return false, nil
	}

	client, err := s.sshConnect(node)
	if err != nil {
		return false, fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	remoteContent, err := client.ReadRemoteFile(xrayConfigPath)
	if err != nil {
		return false, fmt.Errorf("读取远端配置失败: %w", err)
	}

	remoteHash := ConfigHash(remoteContent)
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
			continue // 从未同步，跳过
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

// buildConfig 生成节点 Xray 配置内容
func (s *SyncService) buildConfig(node *entity.Node) (string, error) {
	users, err := s.userRepo.FindActiveUsersByNodeID(node.ID)
	if err != nil {
		return "", fmt.Errorf("查询节点用户失败: %w", err)
	}
	return s.xrayCfgSvc.GenerateConfig(node, users)
}

// pushConfig 上传配置到节点并重载 xray
func (s *SyncService) pushConfig(node *entity.Node, content string) error {
	client, err := s.sshConnect(node)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.UploadContent(content, xrayConfigPath); err != nil {
		return fmt.Errorf("上传配置失败: %w", err)
	}
	if err := client.ReloadXray(); err != nil {
		return fmt.Errorf("重载 xray 失败: %w", err)
	}
	return nil
}

// sshConnect 根据节点配置建立 SSH 连接
func (s *SyncService) sshConnect(node *entity.Node) (*xssh.Client, error) {
	sshPort := node.SSHPort
	if sshPort == 0 {
		sshPort = 22
	}
	sshUser := node.SSHUser
	if sshUser == "" {
		sshUser = "root"
	}
	return xssh.Connect(xssh.Config{
		Host:    node.IP,
		Port:    sshPort,
		User:    sshUser,
		KeyPath: node.SSHKeyPath,
	})
}

// failNode 将节点标记为同步失败并记录日志
func (s *SyncService) failNode(node *entity.Node, errMsg string, elapsedMs int64) {
	_ = s.nodeRepo.UpdateSyncStatus(node.ID, entity.SyncStatusFailed, "")
	s.logRepo.Record(
		"sync",
		fmt.Sprintf("node:%s(%d)", node.Name, node.ID),
		false,
		errMsg,
		elapsedMs,
	)
}
