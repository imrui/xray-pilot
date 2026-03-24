package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

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
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败: %w", err)
	}
	user := &entity.User{
		Username:     req.Username,
		PasswordHash: string(hash),
		RealName:     req.RealName,
		Department:   req.Department,
		GroupID:      req.GroupID,
		Remark:       req.Remark,
		UUID:         uuid.NewString(),
		Token:        uuid.NewString(),
		Active:       true,
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
	if req.Department != "" {
		user.Department = req.Department
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
	// 非空时更新密码
	if req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("密码加密失败: %w", err)
		}
		user.PasswordHash = string(hash)
	}
	if err := s.userRepo.Update(user); err != nil {
		return nil, err
	}
	return s.toResponse(user, baseURL), nil
}

func (s *UserService) Delete(id uint) error {
	return s.userRepo.Delete(id)
}

// DisableUser 禁用用户
// 注意：禁用用户后需异步触发全量节点同步，并更新所有节点 SyncStatus 为 drifted
// TODO: 在此处发布异步事件，由 SyncService 消费并执行全量同步
func (s *UserService) DisableUser(id uint) error {
	return s.userRepo.UpdateActive(id, false)
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

func (s *UserService) toResponse(u *entity.User, baseURL string) *dto.UserResponse {
	resp := &dto.UserResponse{
		ID:           u.ID,
		Username:     u.Username,
		RealName:     u.RealName,
		Department:   u.Department,
		GroupID:      u.GroupID,
		Active:       u.Active,
		Remark:       u.Remark,
		SubscribeURL: fmt.Sprintf("%s/sub/%s", baseURL, u.Token),
		CreatedAt:    u.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    u.UpdatedAt.Format(time.RFC3339),
	}
	if u.Group != nil {
		resp.GroupName = u.Group.Name
	}
	return resp
}
