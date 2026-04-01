package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

type UserService struct {
	userRepo *repository.UserRepository
	nodeRepo *repository.NodeRepository
}

func NewUserService() *UserService {
	return &UserService{
		userRepo: repository.NewUserRepository(),
		nodeRepo: repository.NewNodeRepository(),
	}
}

func (s *UserService) Create(req *dto.CreateUserRequest, baseURL string) (*dto.UserResponse, error) {
	var feishuBoundAt *time.Time
	if req.FeishuOpenID != "" || req.FeishuUnionID != "" || req.FeishuChatID != "" {
		now := time.Now()
		feishuBoundAt = &now
	}

	user := &entity.User{
		Username:      req.Username,
		RealName:      req.RealName,
		GroupID:       req.GroupID,
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
	if err := s.userRepo.Create(user); err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}
	_ = s.nodeRepo.MarkAllDrifted()
	return s.toResponse(user, baseURL), nil
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
	if req.GroupID != nil {
		groupID, err := parseOptionalUint(req.GroupID)
		if err != nil {
			return nil, fmt.Errorf("解析分组失败: %w", err)
		}
		if (user.GroupID == nil) != (groupID == nil) || (user.GroupID != nil && groupID != nil && *user.GroupID != *groupID) {
			shouldMarkSync = true
		}
		user.GroupID = groupID
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
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	if shouldMarkSync {
		_ = s.nodeRepo.MarkAllDrifted()
	}
	return s.toResponse(user, baseURL), nil
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
	result := make([]dto.UserResponse, 0, len(users))
	for i := range users {
		result = append(result, *s.toResponse(&users[i], baseURL))
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
	resp := &dto.UserResponse{
		ID:            u.ID,
		Username:      u.Username,
		RealName:      u.RealName,
		GroupID:       u.GroupID,
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
	if u.Group != nil {
		resp.GroupName = u.Group.Name
	}
	if u.FeishuBoundAt != nil {
		resp.FeishuBoundAt = u.FeishuBoundAt.Format(time.RFC3339)
	}
	return resp
}

func parseOptionalUint(raw json.RawMessage) (*uint, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var value uint
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	return &value, nil
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
