package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

type UserService struct {
	userRepo    *repository.UserRepository
	nodeRepo    *repository.NodeRepository
	trafficRepo *repository.TrafficRepository
}

func NewUserService() *UserService {
	return &UserService{
		userRepo:    repository.NewUserRepository(),
		nodeRepo:    repository.NewNodeRepository(),
		trafficRepo: repository.NewTrafficRepository(),
	}
}

func (s *UserService) Create(req *dto.CreateUserRequest, baseURL string) (*dto.UserResponse, error) {
	var feishuBoundAt *time.Time
	if req.FeishuOpenID != "" || req.FeishuUnionID != "" || req.FeishuChatID != "" {
		now := time.Now()
		feishuBoundAt = &now
	}

	user := &entity.User{
		Username:      strings.TrimSpace(req.Username),
		RealName:      req.RealName,
		Remark:        req.Remark,
		FeishuEnabled: req.FeishuEnabled,
		FeishuEmail:   strings.ToLower(strings.TrimSpace(req.FeishuEmail)),
		UUID:          uuid.NewString(),
		Token:         uuid.NewString(),
		Active:        true,
		ExpiresAt:     req.ExpiresAt,
		FeishuOpenID:  req.FeishuOpenID,
		FeishuUnionID: req.FeishuUnionID,
		FeishuChatID:  req.FeishuChatID,
		FeishuBoundAt: feishuBoundAt,
	}
	if user.Username == "" {
		return nil, errors.New("用户名不能为空")
	}
	if err := repository.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return fmt.Errorf("创建用户失败: %w", err)
		}
		if err := s.userRepo.ReplaceGroupsTx(tx, user, req.GroupIDs); err != nil {
			return fmt.Errorf("保存用户分组失败: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	created, err := s.userRepo.FindByID(user.ID)
	if err != nil {
		return nil, fmt.Errorf("读取用户失败: %w", err)
	}
	_ = s.nodeRepo.MarkAllDrifted()
	return s.toResponse(created, baseURL), nil
}

func (s *UserService) Update(id uint, req *dto.UpdateUserRequest, baseURL string) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	shouldMarkSync := false
	if req.Username != nil {
		username := strings.TrimSpace(*req.Username)
		if username == "" {
			return nil, fmt.Errorf("用户名不能为空")
		}
		if user.Username != username {
			shouldMarkSync = true
		}
		user.Username = username
	}
	if req.RealName != nil {
		user.RealName = *req.RealName
	}
	if req.GroupIDs != nil {
		groupIDs, err := parseOptionalUintSlice(req.GroupIDs)
		if err != nil {
			return nil, fmt.Errorf("解析分组失败: %w", err)
		}
		currentGroupIDs := extractGroupIDs(user.Groups)
		if !slices.Equal(currentGroupIDs, groupIDs) {
			shouldMarkSync = true
		}
	}
	if req.Remark != nil {
		user.Remark = *req.Remark
	}
	if req.FeishuEnabled != nil {
		user.FeishuEnabled = *req.FeishuEnabled
	}
	if req.FeishuEmail != nil {
		user.FeishuEmail = strings.ToLower(strings.TrimSpace(*req.FeishuEmail))
	}
	if req.FeishuOpenID != nil {
		user.FeishuOpenID = *req.FeishuOpenID
	}
	if req.FeishuUnionID != nil {
		user.FeishuUnionID = *req.FeishuUnionID
	}
	if req.FeishuChatID != nil {
		user.FeishuChatID = *req.FeishuChatID
	}
	if req.Active != nil {
		if user.Active != *req.Active {
			shouldMarkSync = true
		}
		user.Active = *req.Active
	}
	if req.ExpiresAt != nil {
		expiresAt, err := parseOptionalTime(req.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("解析过期时间失败: %w", err)
		}
		if (user.ExpiresAt == nil) != (expiresAt == nil) || (user.ExpiresAt != nil && expiresAt != nil && !user.ExpiresAt.Equal(*expiresAt)) {
			shouldMarkSync = true
		}
		user.ExpiresAt = expiresAt
	}
	if user.FeishuOpenID != "" || user.FeishuUnionID != "" || user.FeishuChatID != "" {
		now := time.Now()
		user.FeishuBoundAt = &now
	} else {
		user.FeishuBoundAt = nil
	}
	if err := repository.DB.Transaction(func(tx *gorm.DB) error {
		if req.GroupIDs != nil {
			groupIDs, err := parseOptionalUintSlice(req.GroupIDs)
			if err != nil {
				return fmt.Errorf("解析分组失败: %w", err)
			}
			if err := s.userRepo.ReplaceGroupsTx(tx, user, groupIDs); err != nil {
				return fmt.Errorf("更新用户分组失败: %w", err)
			}
		}
		if err := tx.Save(user).Error; err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	updated, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("读取用户失败: %w", err)
	}
	if shouldMarkSync {
		_ = s.nodeRepo.MarkAllDrifted()
	}
	return s.toResponse(updated, baseURL), nil
}

func (s *UserService) Delete(id uint) error {
	if err := s.userRepo.Delete(id); err != nil {
		return err
	}
	return s.nodeRepo.MarkAllDrifted()
}

func (s *UserService) ToggleActive(id uint) error {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return errors.New("用户不存在")
	}
	if err := s.userRepo.UpdateActive(id, !user.Active); err != nil {
		return err
	}
	return s.nodeRepo.MarkAllDrifted()
}

func (s *UserService) List(page, pageSize int, baseURL string) ([]dto.UserResponse, int64, error) {
	users, total, err := s.userRepo.List(page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	// 一次性 join 当页用户的累计流量，避免 N+1
	ids := make([]uint, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	totals, _ := s.trafficRepo.ListTotalsByUserIDs(ids) // 失败时空 map，不阻塞主流程

	result := make([]dto.UserResponse, 0, len(users))
	for i := range users {
		resp := s.toResponse(&users[i], baseURL)
		if t, ok := totals[users[i].ID]; ok {
			resp.TrafficUpBytes = t.UpBytes
			resp.TrafficDownBytes = t.DownBytes
			if !t.LastUpdatedAt.IsZero() {
				resp.TrafficLastUpdatedAt = t.LastUpdatedAt.Format(time.RFC3339)
			}
		}
		result = append(result, *resp)
	}
	return result, total, nil
}

// ResetUUID 重置用户 UUID（触发全节点重新同步）
func (s *UserService) ResetUUID(id uint, baseURL string) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	newUUID := uuid.NewString()
	if err := s.userRepo.UpdateUUID(id, newUUID); err != nil {
		return nil, fmt.Errorf("重置 UUID 失败: %w", err)
	}
	user.UUID = newUUID
	_ = s.nodeRepo.MarkAllDrifted()
	return s.toResponse(user, baseURL), nil
}

// ResetToken 重置用户订阅 Token
func (s *UserService) ResetToken(id uint, baseURL string) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	newToken := uuid.NewString()
	if err := s.userRepo.UpdateToken(id, newToken); err != nil {
		return nil, fmt.Errorf("重置 Token 失败: %w", err)
	}
	user.Token = newToken
	return s.toResponse(user, baseURL), nil
}

func (s *UserService) toResponse(u *entity.User, baseURL string) *dto.UserResponse {
	groupIDs := extractGroupIDs(u.Groups)
	groupNames := extractGroupNames(u.Groups)
	groups := make([]dto.UserGroupSummary, 0, len(u.Groups))
	for _, group := range u.Groups {
		groups = append(groups, dto.UserGroupSummary{
			ID:   group.ID,
			Name: group.Name,
		})
	}

	resp := &dto.UserResponse{
		ID:            u.ID,
		Username:      u.Username,
		RealName:      u.RealName,
		GroupIDs:      groupIDs,
		GroupNames:    groupNames,
		Groups:        groups,
		Active:        u.Active,
		Remark:        u.Remark,
		SubscribeURL:  fmt.Sprintf("%s/sub/%s", baseURL, u.Token),
		FeishuEnabled: u.FeishuEnabled,
		FeishuEmail:   u.FeishuEmail,
		FeishuOpenID:  u.FeishuOpenID,
		FeishuUnionID: u.FeishuUnionID,
		FeishuChatID:  u.FeishuChatID,
		CreatedAt:     u.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     u.UpdatedAt.Format(time.RFC3339),
	}
	if u.ExpiresAt != nil {
		resp.ExpiresAt = u.ExpiresAt.Format(time.RFC3339)
	}
	if u.FeishuBoundAt != nil {
		resp.FeishuBoundAt = u.FeishuBoundAt.Format(time.RFC3339)
	}
	return resp
}

func parseOptionalUintSlice(raw json.RawMessage) ([]uint, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var values []uint
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	return uniqueSortedIDs(values), nil
}

func parseOptionalTime(raw json.RawMessage) (*time.Time, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == "" {
		return nil, nil
	}

	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02 15:04",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return &parsed, nil
		}
	}
	return nil, fmt.Errorf("unsupported time format: %s", value)
}

func extractGroupIDs(groups []entity.Group) []uint {
	if len(groups) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	return uniqueSortedIDs(ids)
}

func extractGroupNames(groups []entity.Group) []string {
	if len(groups) == 0 {
		return nil
	}
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func uniqueSortedIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}
	cloned := append([]uint(nil), ids...)
	slices.Sort(cloned)
	return slices.Compact(cloned)
}
