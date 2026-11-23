package service

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
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
	// 文本字段统一去首尾空格：域名/IP 混入空格会生成错误的订阅地址且难以肉眼发现
	node := &entity.Node{
		Name:       strings.TrimSpace(req.Name),
		Region:     strings.TrimSpace(req.Region),
		Owner:      strings.TrimSpace(req.Owner),
		IP:         strings.TrimSpace(req.IP),
		Domain:     strings.TrimSpace(req.Domain),
		SSHPort:    req.SSHPort,
		SSHUser:    strings.TrimSpace(req.SSHUser),
		SSHKeyPath: strings.TrimSpace(req.SSHKeyPath),
		LogLevel:   req.LogLevel,
		Remark:     strings.TrimSpace(req.Remark),
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

func (s *NodeService) Update(id uint, req *dto.UpdateNodeRequest, actor string) (*dto.NodeResponse, error) {
	wakeAfterSave := false
	unlock := lockNode(id)
	defer func() {
		unlock()
		if wakeAfterSave {
			wakeDNS()
		}
	}()
	fields := map[string]any{}
	for key, value := range map[string]*string{
		"name": req.Name, "region": req.Region, "owner": req.Owner, "remark": req.Remark,
		"ip": req.IP, "domain": req.Domain, "ssh_user": req.SSHUser, "ssh_key_path": req.SSHKeyPath,
	} {
		if value != nil {
			fields[key] = strings.TrimSpace(*value)
		}
	}
	if req.LogLevel != nil {
		fields["log_level"] = *req.LogLevel
	}
	if req.SSHPort != nil && *req.SSHPort != 0 {
		fields["ssh_port"] = *req.SSHPort
	}
	old, node, err := s.nodeRepo.Update(id, fields, actor)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("节点不存在")
	}
	if err != nil {
		return nil, err
	}
	wakeAfterSave = old.IP != node.IP || old.Domain != node.Domain
	if old.IP != node.IP && old.IP != "" {
		addr := net.JoinHostPort(old.IP, strconv.Itoa(effectiveSSHPort(&old, s.settingSvc)))
		if err := xssh.RemoveKnownHost(s.settingSvc.Get(KeySSHKnownHostsPath), addr); err != nil {
			zap.L().Warn("清理 known_hosts 旧条目失败", zap.String("addr", addr), zap.Error(err))
		}
	}
	return s.nodeWithDNS(&node)
}

func (s *NodeService) Delete(id uint) error {
	dnsControls.Lock()
	defer dnsControls.Unlock()
	defer lockNode(id)()
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
	return s.nodeWithDNS(node)
}

func (s *NodeService) nodeWithDNS(n *entity.Node) (*dto.NodeResponse, error) {
	resp := s.toNodeResponse(n)
	binding, err := NewDNSService().Binding(n.ID)
	if err != nil {
		return nil, err
	}
	resp.DNSBinding = binding
	return resp, nil
}

func (s *NodeService) List(page, pageSize int, filter repository.NodeListFilter) ([]dto.NodeResponse, int64, error) {
	nodes, total, err := s.nodeRepo.List(page, pageSize, filter)
	if err != nil {
		return nil, 0, err
	}
	result := make([]dto.NodeResponse, 0, len(nodes))
	ids := make([]uint, 0, len(nodes))
	for i := range nodes {
		ids = append(ids, nodes[i].ID)
	}
	var bindings []entity.DNSBinding
	if len(ids) > 0 {
		if err := repository.DB.Where("node_id IN ?", ids).Find(&bindings).Error; err != nil {
			return nil, 0, err
		}
	}
	byNode := make(map[uint]*entity.DNSBinding, len(bindings))
	for i := range bindings {
		byNode[bindings[i].NodeID] = &bindings[i]
	}
	for i := range nodes {
		resp := s.toNodeResponse(&nodes[i])
		resp.DNSBinding = byNode[nodes[i].ID]
		result = append(result, *resp)
	}
	return result, total, nil
}

// FilterOptions 返回筛选下拉所需的全量地区/所有者候选值。
// 单独提供而非塞进 List 响应：下拉选项不应随筛选条件和分页变化。
func (s *NodeService) FilterOptions() (regions, owners []string, err error) {
	regions, err = s.nodeRepo.DistinctRegions()
	if err != nil {
		return nil, nil, err
	}
	owners, err = s.nodeRepo.DistinctOwners()
	if err != nil {
		return nil, nil, err
	}
	return regions, owners, nil
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
		Owner:           n.Owner,
		IP:              n.IP,
		Domain:          n.Domain,
		GroupNames:      groupNames,
		OnlineUserCount: int(onlineUsers),
		SSHPort:         n.SSHPort,
		SSHUser:         n.SSHUser,
		SSHKeyPath:      n.SSHKeyPath,
		LogLevel:        n.LogLevel,
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
		resp.LastCheckAt = n.LastCheckAt.Format(time.RFC3339Nano)
	}
	return resp
}
