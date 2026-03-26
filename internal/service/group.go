package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
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
	if err := s.groupRepo.Create(group); err != nil {
		return nil, fmt.Errorf("创建分组失败: %w", err)
	}
	if len(req.NodeIDs) > 0 {
		nodes, err := s.nodeRepo.FindByIDs(req.NodeIDs)
		if err == nil {
			_ = s.groupRepo.ReplaceNodes(group, nodes)
			// 新关联的节点需要同步新分组的用户配置
			_ = s.nodeRepo.BatchUpdateSyncStatus(req.NodeIDs, entity.SyncStatusDrifted)
		}
	}
	return s.toResponse(group), nil
}

// UpdateGroup 更新分组：节点关联变更后自动标记受影响节点为漂移状态
func (s *GroupService) UpdateGroup(id uint, req *dto.UpdateGroupRequest) (*dto.GroupResponse, error) {
	group, err := s.groupRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("分组不存在")
	}
	if req.Name != "" {
		group.Name = req.Name
	}
	if req.Description != "" {
		group.Description = req.Description
	}
	if err := s.groupRepo.Update(group); err != nil {
		return nil, err
	}
	if req.NodeIDs != nil {
		// 记录变更前的旧节点 ID
		oldIDs := make([]uint, 0, len(group.Nodes))
		for _, n := range group.Nodes {
			oldIDs = append(oldIDs, n.ID)
		}

		nodes, err := s.nodeRepo.FindByIDs(req.NodeIDs)
		if err == nil {
			_ = s.groupRepo.ReplaceNodes(group, nodes)
		}

		// 旧节点 ∪ 新节点均需重新同步（旧节点移除了该分组用户，新节点增加了该分组用户）
		affected := unionUintSlices(oldIDs, req.NodeIDs)
		_ = s.nodeRepo.BatchUpdateSyncStatus(affected, entity.SyncStatusDrifted)
	}
	return s.toResponse(group), nil
}

func (s *GroupService) Delete(id uint) error {
	return s.groupRepo.Delete(id)
}

func (s *GroupService) ToggleActive(id uint) error {
	group, err := s.groupRepo.FindByID(id)
	if err != nil {
		return errors.New("分组不存在")
	}
	return s.groupRepo.UpdateActive(id, !group.Active)
}

func (s *GroupService) List(page, pageSize int) ([]dto.GroupResponse, int64, error) {
	groups, total, err := s.groupRepo.List(page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	var result []dto.GroupResponse
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
