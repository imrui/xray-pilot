package service

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/repository"
)

type AuthService struct {
	userRepo *repository.UserRepository
}

func NewAuthService() *AuthService {
	return &AuthService{userRepo: repository.NewUserRepository()}
}

// Login 用户登录，bcrypt 校验密码后返回 JWT Token
func (s *AuthService) Login(req *dto.LoginRequest) (*dto.LoginResponse, error) {
	user, err := s.userRepo.FindByUsername(req.Username)
	if err != nil {
		// 用户名不存在时返回与密码错误相同的提示，防止用户枚举
		return nil, errors.New("用户名或密码错误")
	}
	if !user.Active {
		return nil, errors.New("账号已禁用")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, errors.New("用户名或密码错误")
	}
	token, err := generateJWT(user.ID, user.Username)
	if err != nil {
		return nil, errors.New("生成 Token 失败")
	}
	return &dto.LoginResponse{Token: token}, nil
}

func generateJWT(userID uint, username string) (string, error) {
	expire := time.Duration(config.Global.JWT.Expire) * time.Hour
	claims := jwt.MapClaims{
		"sub":      userID,
		"username": username,
		"exp":      time.Now().Add(expire).Unix(),
		"iat":      time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.Global.JWT.Secret))
}
