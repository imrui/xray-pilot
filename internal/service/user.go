package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

type UserService struct {
	userRepo *repository.UserRepository
}

func NewUserService() *UserService {
	return &UserService{userRepo: repository.NewUserRepository()}
}

func (s *UserService) Create(req *dto.CreateUserRequest, baseURL string) (*dto.UserResponse, error) {
	user := &entity.User{
		Username:  req.Username,
		RealName:  req.RealName,
		GroupID:   req.GroupID,
		Remark:    req.Remark,
		UUID:      uuid.NewString(),
		Token:     uuid.NewString(),
		Active:    true,
		ExpiresAt: req.ExpiresAt,
	}
	if err := s.userRepo.Create(user); err != nil {
		return nil, fmt.Errorf("创建用户失败: %w", err)
	}
	return s.toResponse(user, baseURL), nil
}

func (s *UserService) Update(id uint, req *dto.UpdateUserRequest, baseURL string) (*dto.UserResponse, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, errors.New("用户不存在")
	}
	if req.RealName != "" {
		user.RealName = req.RealName
	}
	if req.GroupID != nil {
		user.GroupID = req.GroupID
	}
	if req.Remark != "" {
		user.Remark = req.Remark
	}
	if req.Active != nil {
		user.Active = *req.Active
	}
	if req.ExpiresAt != nil {
		user.ExpiresAt = req.ExpiresAt
	}
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	return s.toResponse(user, baseURL), nil
}

func (s *UserService) Delete(id uint) error {
	return s.userRepo.Delete(id)
}

func (s *UserService) ToggleActive(id uint) error {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return errors.New("用户不存在")
	}
	return s.userRepo.UpdateActive(id, !user.Active)
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
		ID:           u.ID,
		Username:     u.Username,
		RealName:     u.RealName,
		GroupID:      u.GroupID,
		Active:       u.Active,
		Remark:       u.Remark,
		SubscribeURL: fmt.Sprintf("%s/sub/%s", baseURL, u.Token),
		CreatedAt:    u.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    u.UpdatedAt.Format(time.RFC3339),
	}
	if u.ExpiresAt != nil {
		resp.ExpiresAt = u.ExpiresAt.Format(time.RFC3339)
	}
	if u.Group != nil {
		resp.GroupName = u.Group.Name
	}
	return resp
}
