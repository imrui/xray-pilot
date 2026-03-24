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
		}
	}
	return s.toResponse(group), nil
}

// UpdateGroup 更新分组
// 注意：变更分组节点后，需重新计算受影响节点的 ConfigHash 对比，标记漂移
// TODO: 节点变更后调用 ConfigHashService 重算并对比，对有差异的节点置 SyncStatus=drifted
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
		nodes, err := s.nodeRepo.FindByIDs(req.NodeIDs)
		if err == nil {
			_ = s.groupRepo.ReplaceNodes(group, nodes)
			// TODO: 触发受影响节点的漂移检测
		}
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
	resp := &dto.GroupResponse{
		ID:          g.ID,
		Name:        g.Name,
		Description: g.Description,
		Active:      g.Active,
		NodeCount:   len(g.Nodes),
		CreatedAt:   g.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   g.UpdatedAt.Format(time.RFC3339),
	}
	return resp
}
