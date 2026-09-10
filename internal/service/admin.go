package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

// AdminService 管理员账号管理（v0.5.0）
//
// 角色边界（硬编码两级，不做 RBAC）：
//   - super_admin：全部业务操作 + 管理员增删改 + 重置他人密码
//   - admin：全部业务操作，仅可改自己密码
//
// 护栏：不能删除 / 降级 / 禁用最后一个启用态 super_admin；对自己不能删除、禁用、改角色或重置密码（自己只能走 ChangeOwnPassword 改密）。
type AdminService struct {
	repo    *repository.AdminRepository
	logRepo *repository.LogRepository
}

func NewAdminService() *AdminService {
	return &AdminService{
		repo:    repository.NewAdminRepository(),
		logRepo: repository.NewLogRepository(),
	}
}

const minAdminPasswordLen = 6

// ErrLastSuperAdmin 触发「最后一个超级管理员」护栏
var ErrLastSuperAdmin = errors.New("至少需要保留一个启用的超级管理员")

// SeedFromConfig 首次启动把 config.yaml admins 种入数据库（表非空则忽略 config）。
// 第一个种子账号为 super_admin，其余为 admin。config 中的密码已在 config.Load 阶段 bcrypt。
func (s *AdminService) SeedFromConfig() error {
	count, err := s.repo.Count()
	if err != nil {
		return fmt.Errorf("查询管理员数量失败: %w", err)
	}
	if count > 0 {
		if len(config.Global.Admins) > 0 {
			zap.L().Info("管理员账号已入库，config.yaml 中的 admins 段仅作首次种子，本次忽略")
		}
		return nil
	}
	for i, a := range config.Global.Admins {
		username := strings.TrimSpace(a.Username)
		if username == "" || a.PasswordHash == "" {
			continue
		}
		role := entity.RoleAdmin
		if i == 0 {
			role = entity.RoleSuperAdmin
		}
		if err := s.repo.Create(&entity.AdminUser{
			Username:     username,
			PasswordHash: a.PasswordHash,
			Role:         role,
			Active:       true,
		}); err != nil {
			return fmt.Errorf("种入管理员 %s 失败: %w", username, err)
		}
		zap.L().Info("管理员账号已从 config.yaml 种入数据库", zap.String("username", username), zap.String("role", role))
	}
	return nil
}

func (s *AdminService) List() ([]dto.AdminResponse, error) {
	admins, err := s.repo.List()
	if err != nil {
		return nil, err
	}
	out := make([]dto.AdminResponse, 0, len(admins))
	for i := range admins {
		out = append(out, toAdminResponse(&admins[i]))
	}
	return out, nil
}

func (s *AdminService) Create(req *dto.CreateAdminRequest, actor string) (*dto.AdminResponse, error) {
	username := strings.TrimSpace(req.Username)
	if username == "" {
		return nil, errors.New("用户名不能为空")
	}
	if err := validatePassword(req.Password); err != nil {
		return nil, err
	}
	role := req.Role
	if role == "" {
		role = entity.RoleAdmin
	}
	if !entity.ValidAdminRole(role) {
		return nil, fmt.Errorf("无效的角色: %s", role)
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		return nil, err
	}
	admin := &entity.AdminUser{Username: username, PasswordHash: hash, Role: role, Active: true}
	if err := s.repo.Create(admin); err != nil {
		return nil, fmt.Errorf("创建管理员失败（用户名可能已存在）: %w", err)
	}
	s.logRepo.RecordWithActor("admin_create", adminTarget(admin), actor, true, "角色 "+role, 0)
	resp := toAdminResponse(admin)
	return &resp, nil
}

// Update 修改角色 / 启停。selfID 为当前操作者：不能改自己的角色或禁用自己。
func (s *AdminService) Update(id uint, req *dto.UpdateAdminRequest, selfID uint, actor string) (*dto.AdminResponse, error) {
	admin, err := s.repo.FindByID(id)
	if err != nil {
		return nil, errors.New("管理员不存在")
	}
	changes := make([]string, 0, 2)
	if req.Role != nil && *req.Role != admin.Role {
		if admin.ID == selfID {
			return nil, errors.New("不能修改自己的角色")
		}
		if !entity.ValidAdminRole(*req.Role) {
			return nil, fmt.Errorf("无效的角色: %s", *req.Role)
		}
		if admin.IsSuperAdmin() {
			if err := s.ensureNotLastSuperAdmin(admin.ID); err != nil {
				return nil, err
			}
		}
		admin.Role = *req.Role
		changes = append(changes, "角色→"+admin.Role)
	}
	if req.Active != nil && *req.Active != admin.Active {
		if !*req.Active {
			if admin.ID == selfID {
				return nil, errors.New("不能禁用自己")
			}
			if admin.IsSuperAdmin() {
				if err := s.ensureNotLastSuperAdmin(admin.ID); err != nil {
					return nil, err
				}
			}
		}
		admin.Active = *req.Active
		if admin.Active {
			changes = append(changes, "启用")
		} else {
			changes = append(changes, "禁用")
		}
	}
	if len(changes) == 0 {
		resp := toAdminResponse(admin)
		return &resp, nil
	}
	if err := s.repo.Update(admin); err != nil {
		return nil, fmt.Errorf("更新管理员失败: %w", err)
	}
	s.logRepo.RecordWithActor("admin_update", adminTarget(admin), actor, true, strings.Join(changes, "，"), 0)
	resp := toAdminResponse(admin)
	return &resp, nil
}

func (s *AdminService) Delete(id uint, selfID uint, actor string) error {
	if id == selfID {
		return errors.New("不能删除自己")
	}
	admin, err := s.repo.FindByID(id)
	if err != nil {
		return errors.New("管理员不存在")
	}
	if admin.IsSuperAdmin() && admin.Active {
		if err := s.ensureNotLastSuperAdmin(admin.ID); err != nil {
			return err
		}
	}
	if err := s.repo.Delete(id); err != nil {
		return fmt.Errorf("删除管理员失败: %w", err)
	}
	s.logRepo.RecordWithActor("admin_delete", adminTarget(admin), actor, true, "", 0)
	return nil
}

// SetPassword 超级管理员重置他人密码（不校验旧密码）。selfID 为当前操作者，自己的密码只能走 ChangeOwnPassword。
func (s *AdminService) SetPassword(id uint, password string, selfID uint, actor string) error {
	if id == selfID {
		return errors.New("不能重置自己的密码，请在头像菜单中修改")
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	admin, err := s.repo.FindByID(id)
	if err != nil {
		return errors.New("管理员不存在")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePasswordHash(id, hash); err != nil {
		return fmt.Errorf("重置密码失败: %w", err)
	}
	s.logRepo.RecordWithActor("admin_reset_password", adminTarget(admin), actor, true, "", 0)
	return nil
}

// ChangeOwnPassword 修改自己的密码（需校验旧密码）
func (s *AdminService) ChangeOwnPassword(id uint, req *dto.ChangePasswordRequest, actor string) error {
	admin, err := s.repo.FindByID(id)
	if err != nil {
		return errors.New("管理员不存在")
	}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.OldPassword)) != nil {
		return errors.New("旧密码错误")
	}
	if err := validatePassword(req.NewPassword); err != nil {
		return err
	}
	hash, err := hashPassword(req.NewPassword)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePasswordHash(id, hash); err != nil {
		return fmt.Errorf("修改密码失败: %w", err)
	}
	s.logRepo.RecordWithActor("admin_change_password", adminTarget(admin), actor, true, "", 0)
	return nil
}

func (s *AdminService) ensureNotLastSuperAdmin(excludeID uint) error {
	others, err := s.repo.CountActiveSuperAdminsExcluding(excludeID)
	if err != nil {
		return fmt.Errorf("校验超级管理员数量失败: %w", err)
	}
	if others == 0 {
		return ErrLastSuperAdmin
	}
	return nil
}

func validatePassword(pw string) error {
	if len(pw) < minAdminPasswordLen {
		return fmt.Errorf("密码长度至少 %d 位", minAdminPasswordLen)
	}
	return nil
}

func hashPassword(pw string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("密码哈希失败: %w", err)
	}
	return string(hash), nil
}

func adminTarget(a *entity.AdminUser) string {
	return fmt.Sprintf("admin:%s(%d)", a.Username, a.ID)
}

func toAdminResponse(a *entity.AdminUser) dto.AdminResponse {
	resp := dto.AdminResponse{
		ID:        a.ID,
		Username:  a.Username,
		Role:      a.Role,
		Active:    a.Active,
		CreatedAt: a.CreatedAt.Format(time.RFC3339),
	}
	if a.LastLoginAt != nil {
		resp.LastLoginAt = a.LastLoginAt.Format(time.RFC3339)
	}
	return resp
}
