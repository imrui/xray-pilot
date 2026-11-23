package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"gorm.io/gorm"
)

type GroupService struct {
	groupRepo *repository.GroupRepository
	nodeRepo  *repository.NodeRepository
}

func NewGroupService() *GroupService {
	return &GroupService{
		groupRepo: repository.NewGroupRepository(),
		nodeRepo:  repository.NewNodeRepository(),
	}
}

func (s *GroupService) Create(req *dto.CreateGroupRequest) (*dto.GroupResponse, error) {
	group := &entity.Group{
		Name:        req.Name,
		Description: req.Description,
		Active:      true,
	}
	if len(req.NodeIDs) > 0 {
		nodes, err := s.nodeRepo.FindByIDs(req.NodeIDs)
		if err != nil {
			return nil, err
		}
		if len(nodes) != len(unionUintSlices(nil, req.NodeIDs)) {
			return nil, errors.New("选择的节点不存在，请刷新后重试")
		}
		group.Nodes = nodes
	}
	// A new group has neither users nor a routing binding; no node configuration changes yet.
	if err := s.groupRepo.Create(group); err != nil {
		return nil, fmt.Errorf("创建分组失败: %w", err)
	}
	return s.toResponse(group), nil
}

// UpdateGroup 更新分组：节点关联变更后自动标记受影响节点为漂移状态
func (s *GroupService) UpdateGroup(id uint, req *dto.UpdateGroupRequest) (*dto.GroupResponse, error) {
	fields := map[string]any{}
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		fields["name"] = *req.Name
	}
	if req.Description != nil {
		fields["description"] = *req.Description
	}
	if req.Active != nil {
		fields["active"] = *req.Active
	}
	if err := s.groupRepo.Update(id, fields, req.NodeIDs); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("分组不存在")
		}
		return nil, err
	}
	group, err := s.groupRepo.FindByID(id)
	if err != nil {
		return nil, err
	}
	return s.toResponse(group), nil
}

func (s *GroupService) Delete(id uint) error {
	return s.groupRepo.Delete(id)
}

func (s *GroupService) ToggleActive(id uint) error {
	err := s.groupRepo.ToggleActive(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.New("分组不存在")
	}
	return err
}

func (s *GroupService) List(page, pageSize int, keyword string, active *bool) ([]dto.GroupResponse, int64, error) {
	groups, total, err := s.groupRepo.List(page, pageSize, keyword, active)
	if err != nil {
		return nil, 0, err
	}
	result := make([]dto.GroupResponse, 0, len(groups))
	for _, g := range groups {
		g := g
		result = append(result, *s.toResponse(&g))
	}
	return result, total, nil
}

func (s *GroupService) toResponse(g *entity.Group) *dto.GroupResponse {
	nodeIDs := make([]uint, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		nodeIDs = append(nodeIDs, n.ID)
	}
	resp := &dto.GroupResponse{
		ID:          g.ID,
		Name:        g.Name,
		Description: g.Description,
		Active:      g.Active,
		NodeCount:   len(g.Nodes),
		NodeIDs:     nodeIDs,
		CreatedAt:   g.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   g.UpdatedAt.Format(time.RFC3339),
	}
	return resp
}

// unionUintSlices 合并两个 uint 切片（去重）
func unionUintSlices(a, b []uint) []uint {
	seen := make(map[uint]struct{}, len(a)+len(b))
	for _, v := range a {
		seen[v] = struct{}{}
	}
	for _, v := range b {
		seen[v] = struct{}{}
	}
	result := make([]uint, 0, len(seen))
	for v := range seen {
		result = append(result, v)
	}
	return result
}
