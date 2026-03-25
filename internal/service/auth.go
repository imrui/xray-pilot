package service

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/dto"
)

// AuthService 鉴权服务（管理员账号由 config.yaml 管理，不依赖数据库）
type AuthService struct{}

func NewAuthService() *AuthService {
	return &AuthService{}
}

// Login 管理员登录，校验 config.yaml admins 列表后返回 JWT Token
func (s *AuthService) Login(req *dto.LoginRequest) (*dto.LoginResponse, error) {
	for _, admin := range config.Global.Admins {
		if admin.Username != req.Username {
			continue
		}
		if admin.PasswordHash == "" {
			return nil, errors.New("用户名或密码错误")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(req.Password)); err != nil {
			return nil, errors.New("用户名或密码错误")
		}
		token, err := generateJWT(admin.Username)
		if err != nil {
			return nil, errors.New("生成 Token 失败")
		}
		return &dto.LoginResponse{Token: token}, nil
	}
	return nil, errors.New("用户名或密码错误")
}

func generateJWT(username string) (string, error) {
	expire := time.Duration(config.Global.JWT.Expire) * time.Hour
	claims := jwt.MapClaims{
		"sub": username,
		"exp": time.Now().Add(expire).Unix(),
		"iat": time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.Global.JWT.Secret))
}
