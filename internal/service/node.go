package service

import (
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/pkg/logger"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

type NodeService struct {
	nodeRepo   *repository.NodeRepository
	groupRepo  *repository.GroupRepository
	logRepo    *repository.LogRepository
	userRepo   *repository.UserRepository
	settingSvc *SettingService
}

func NewNodeService() *NodeService {
	return &NodeService{
		nodeRepo:   repository.NewNodeRepository(),
		groupRepo:  repository.NewGroupRepository(),
		logRepo:    repository.NewLogRepository(),
		userRepo:   repository.NewUserRepository(),
		settingSvc: NewSettingService(),
	}
}

func (s *NodeService) Create(req *dto.CreateNodeRequest) (*dto.NodeResponse, error) {
	node := &entity.Node{
		Name:       req.Name,
		Region:     req.Region,
		IP:         req.IP,
		Domain:     req.Domain,
		SSHPort:    req.SSHPort,
		SSHUser:    req.SSHUser,
		SSHKeyPath: req.SSHKeyPath,
		Remark:     req.Remark,
		Active:     true,
		SyncStatus: entity.SyncStatusPending,
	}
	if node.SSHPort == 0 {
		node.SSHPort = 22
	}
	if node.SSHUser == "" {
		node.SSHUser = "root"
	}
	if err := s.nodeRepo.Create(node); err != nil {
		return nil, fmt.Errorf("创建节点失败: %w", err)
	}
	return s.toNodeResponse(node), nil
}

func (s *NodeService) Update(id uint, req *dto.UpdateNodeRequest) (*dto.NodeResponse, error) {
	node, err := s.nodeRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("节点不存在")
	}

	// 记录变更前的连接地址，用于清理 known_hosts
	oldAddr := node.ConnectAddr()
	ipChanged := req.IP != "" && req.IP != node.IP
	domainChanged := req.Domain != "" && req.Domain != node.Domain

	if req.Name != "" {
		node.Name = req.Name
	}
	if req.Region != "" {
		node.Region = req.Region
	}
	if req.IP != "" {
		node.IP = req.IP
	}
	if req.Domain != "" {
		node.Domain = req.Domain
	}
	if req.SSHPort != 0 {
		node.SSHPort = req.SSHPort
	}
	// SSHUser/SSHKeyPath 允许显式置空，置空后运行时会回退到系统默认设置。
	node.SSHUser = req.SSHUser
	node.SSHKeyPath = req.SSHKeyPath
	if req.Remark != "" {
		node.Remark = req.Remark
	}

	// IP 或 Domain 变更：清理旧的 known_hosts 条目，并标记漂移触发重新同步
	if ipChanged || domainChanged {
		node.SyncStatus = entity.SyncStatusDrifted
		knownHostsPath := s.settingSvc.Get(KeySSHKnownHostsPath)
		if err := xssh.RemoveKnownHost(knownHostsPath, oldAddr); err != nil {
			// 非致命错误，记录警告后继续
			logger.Log.Warn("清理 known_hosts 旧条目失败",
				zap.String("addr", oldAddr),
				zap.Error(err),
			)
		}
	}

	if err := s.nodeRepo.Update(node); err != nil {
		return nil, err
	}
	return s.toNodeResponse(node), nil
}

func (s *NodeService) Delete(id uint) error {
	return s.nodeRepo.Delete(id)
}

func (s *NodeService) ToggleActive(id uint) error {
	node, err := s.nodeRepo.FindByID(id)
	if err != nil {
		return errors.New("节点不存在")
	}
	return s.nodeRepo.UpdateActive(id, !node.Active)
}

func (s *NodeService) GetByID(id uint) (*dto.NodeResponse, error) {
	node, err := s.nodeRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("节点不存在")
	}
	return s.toNodeResponse(node), nil
}

func (s *NodeService) List(page, pageSize int) ([]dto.NodeResponse, int64, error) {
	nodes, total, err := s.nodeRepo.List(page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	result := make([]dto.NodeResponse, 0, len(nodes))
	for i := range nodes {
		result = append(result, *s.toNodeResponse(&nodes[i]))
	}
	return result, total, nil
}

func (s *NodeService) GetDriftedNodes() ([]dto.NodeResponse, error) {
	nodes, err := s.nodeRepo.GetDriftedNodes()
	if err != nil {
		return nil, err
	}
	result := make([]dto.NodeResponse, 0, len(nodes))
	for i := range nodes {
		result = append(result, *s.toNodeResponse(&nodes[i]))
	}
	return result, nil
}

func (s *NodeService) toNodeResponse(n *entity.Node) *dto.NodeResponse {
	groupNames, _ := s.groupRepo.FindNamesByNodeID(n.ID)
	onlineUsers, _ := s.userRepo.CountActiveByNodeID(n.ID)

	resp := &dto.NodeResponse{
		ID:              n.ID,
		Name:            n.Name,
		Region:          n.Region,
		IP:              n.IP,
		Domain:          n.Domain,
		GroupNames:      groupNames,
		OnlineUserCount: int(onlineUsers),
		SSHPort:         n.SSHPort,
		SSHUser:         n.SSHUser,
		SSHKeyPath:      n.SSHKeyPath,
		Active:          n.Active,
		XrayActive:      n.XrayActive,
		XrayVersion:     n.XrayVersion,
		SyncStatus:      string(n.SyncStatus),
		LastCheckOK:     n.LastCheckOK,
		LastLatencyMs:   n.LastLatencyMs,
		Remark:          n.Remark,
		CreatedAt:       n.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       n.UpdatedAt.Format(time.RFC3339),
	}
	if n.LastSyncAt != nil {
		resp.LastSyncAt = n.LastSyncAt.Format(time.RFC3339)
	}
	if n.LastCheckAt != nil {
		resp.LastCheckAt = n.LastCheckAt.Format(time.RFC3339)
	}
	return resp
}
