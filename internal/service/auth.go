package service

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

// JWT claim 键名（handler 中间件与本文件共用）
const (
	ClaimAdminID  = "admin_id"
	ClaimUsername = "username"
	ClaimRole     = "role"
)

// AuthService 鉴权服务（v0.5.0 起管理员账号入库，config.yaml admins 仅作首次种子）
type AuthService struct {
	adminRepo *repository.AdminRepository
	logRepo   *repository.LogRepository
}

func NewAuthService() *AuthService {
	return &AuthService{
		adminRepo: repository.NewAdminRepository(),
		logRepo:   repository.NewLogRepository(),
	}
}

var errBadCredential = errors.New("用户名或密码错误")

// Login 管理员登录：校验账号密码与启用态，返回携带 admin_id / role 的 JWT
func (s *AuthService) Login(req *dto.LoginRequest) (*dto.LoginResponse, error) {
	admin, err := s.adminRepo.FindByUsername(req.Username)
	if err != nil {
		return nil, errBadCredential
	}
	actor := "admin:" + admin.Username
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)) != nil {
		s.logRepo.RecordWithActor("login", actor, actor, false, "密码错误", 0)
		return nil, errBadCredential
	}
	if !admin.Active {
		s.logRepo.RecordWithActor("login", actor, actor, false, "账号已禁用", 0)
		return nil, errors.New("账号已禁用")
	}
	token, err := generateJWT(admin)
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}
	_ = s.adminRepo.UpdateLastLogin(admin.ID, time.Now())
	return &dto.LoginResponse{Token: token, Username: admin.Username, Role: admin.Role}, nil
}

func generateJWT(admin *entity.AdminUser) (string, error) {
	expire := time.Duration(config.Global.JWT.Expire) * time.Hour
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":         admin.Username,
		ClaimAdminID:  admin.ID,
		ClaimUsername: admin.Username,
		ClaimRole:     admin.Role,
		"exp":         now.Add(expire).Unix(),
		"iat":         now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.Global.JWT.Secret))
}
