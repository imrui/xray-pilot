package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/pkg/response"
)

// gin context 键名
const (
	ctxAdminID  = "adminID"
	ctxUsername = "username"
	ctxRole     = "role"
)

// JWTMiddleware JWT Bearer Token 验证中间件。
//
// v0.5.0 起每次请求按 admin_id 回查数据库确认账号仍启用（决策 B1-1：不做 token 黑名单，
// 禁用 / 删除账号后旧 token 立即失效；角色变更靠重新登录刷新 claim）。
// 不带 admin_id 的旧版 token（v0.4.x 签发）直接判为未授权，升级后需重新登录。
func JWTMiddleware() gin.HandlerFunc {
	adminRepo := repository.NewAdminRepository()
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(config.Global.JWT.Secret), nil
		})
		if err != nil || !token.Valid {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			response.Unauthorized(c)
			c.Abort()
			return
		}
		// JSON 数字解析为 float64
		idFloat, ok := claims[service.ClaimAdminID].(float64)
		if !ok || idFloat <= 0 {
			response.Unauthorized(c)
			c.Abort()
			return
		}
		admin, err := adminRepo.FindByID(uint(idFloat))
		if err != nil || !admin.Active {
			response.Unauthorized(c)
			c.Abort()
			return
		}

		c.Set(ctxAdminID, admin.ID)
		c.Set(ctxUsername, admin.Username)
		c.Set(ctxRole, admin.Role)
		c.Next()
	}
}

// RequireSuperAdmin 仅超级管理员可访问（挂在 JWTMiddleware 之后）
func RequireSuperAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(ctxRole) != entity.RoleSuperAdmin {
			response.Forbidden(c, "仅超级管理员可执行此操作")
			c.Abort()
			return
		}
		c.Next()
	}
}

// currentAdminID 当前登录管理员 ID（JWTMiddleware 之后可用）
func currentAdminID(c *gin.Context) uint {
	if v, ok := c.Get(ctxAdminID); ok {
		if id, ok := v.(uint); ok {
			return id
		}
	}
	return 0
}

// actorFrom 当前登录管理员的操作日志 actor 字符串（格式见 entity.SyncLog）
func actorFrom(c *gin.Context) string {
	return "admin:" + c.GetString(ctxUsername)
}
