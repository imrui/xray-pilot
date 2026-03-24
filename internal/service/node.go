package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/pkg/crypto"
)

type NodeService struct {
	nodeRepo *repository.NodeRepository
	logRepo  *repository.LogRepository
}

func NewNodeService() *NodeService {
	return &NodeService{
		nodeRepo: repository.NewNodeRepository(),
		logRepo:  repository.NewLogRepository(),
	}
}

func (s *NodeService) Create(req *dto.CreateNodeRequest) (*dto.NodeResponse, error) {
	encKey, err := encryptKey(req.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("加密私钥失败: %w", err)
	}
	node := &entity.Node{
		Name:       req.Name,
		Region:     req.Region,
		IP:         req.IP,
		Port:       req.Port,
		PrivateKey: encKey,
		PublicKey:  req.PublicKey,
		ShortID:    req.ShortID,
		SNI:        req.SNI,
		SSHPort:    req.SSHPort,
		SSHUser:    req.SSHUser,
		SSHKeyPath: req.SSHKeyPath,
		Remark:     req.Remark,
		Active:     true,
		SyncStatus: entity.SyncStatusPending,
	}
	if node.Port == 0 {
		node.Port = 443
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
	return toNodeResponse(node), nil
}

// UpdateNodeIP 更新节点（含 IP 变更处理）
// 注意：IP 变更后立即将该节点 SyncStatus 置为 drifted，触发单节点同步
func (s *NodeService) UpdateNodeIP(id uint, req *dto.UpdateNodeRequest) (*dto.NodeResponse, error) {
	node, err := s.nodeRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("节点不存在")
	}

	ipChanged := req.IP != "" && req.IP != node.IP

	if req.Name != "" {
		node.Name = req.Name
	}
	if req.Region != "" {
		node.Region = req.Region
	}
	if req.IP != "" {
		node.IP = req.IP
	}
	if req.Port != 0 {
		node.Port = req.Port
	}
	if req.PrivateKey != "" {
		encKey, err := encryptKey(req.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("加密私钥失败: %w", err)
		}
		node.PrivateKey = encKey
	}
	if req.PublicKey != "" {
		node.PublicKey = req.PublicKey
	}
	if req.ShortID != "" {
		node.ShortID = req.ShortID
	}
	if req.SNI != "" {
		node.SNI = req.SNI
	}
	if req.SSHPort != 0 {
		node.SSHPort = req.SSHPort
	}
	if req.SSHUser != "" {
		node.SSHUser = req.SSHUser
	}
	if req.SSHKeyPath != "" {
		node.SSHKeyPath = req.SSHKeyPath
	}
	if req.Remark != "" {
		node.Remark = req.Remark
	}

	if ipChanged {
		node.SyncStatus = entity.SyncStatusDrifted
	}

	if err := s.nodeRepo.Update(node); err != nil {
		return nil, err
	}
	return toNodeResponse(node), nil
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
	return toNodeResponse(node), nil
}

func (s *NodeService) List(page, pageSize int) ([]dto.NodeResponse, int64, error) {
	nodes, total, err := s.nodeRepo.List(page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	result := make([]dto.NodeResponse, 0, len(nodes))
	for i := range nodes {
		result = append(result, *toNodeResponse(&nodes[i]))
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
		result = append(result, *toNodeResponse(&nodes[i]))
	}
	return result, nil
}

// encryptKey 加密 Reality 私钥；空字符串直接返回空
func encryptKey(plainKey string) (string, error) {
	if plainKey == "" {
		return "", nil
	}
	return crypto.Encrypt(plainKey)
}

func toNodeResponse(n *entity.Node) *dto.NodeResponse {
	resp := &dto.NodeResponse{
		ID:            n.ID,
		Name:          n.Name,
		Region:        n.Region,
		IP:            n.IP,
		Port:          n.Port,
		PublicKey:     n.PublicKey,
		ShortID:       n.ShortID,
		SNI:           n.SNI,
		SSHPort:       n.SSHPort,
		SSHUser:       n.SSHUser,
		SSHKeyPath:    n.SSHKeyPath,
		Active:        n.Active,
		SyncStatus:    string(n.SyncStatus),
		LastCheckOK:   n.LastCheckOK,
		LastLatencyMs: n.LastLatencyMs,
		Remark:        n.Remark,
		CreatedAt:     n.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     n.UpdatedAt.Format(time.RFC3339),
	}
	if n.LastSyncAt != nil {
		resp.LastSyncAt = n.LastSyncAt.Format(time.RFC3339)
	}
	if n.LastCheckAt != nil {
		resp.LastCheckAt = n.LastCheckAt.Format(time.RFC3339)
	}
	return resp
}
