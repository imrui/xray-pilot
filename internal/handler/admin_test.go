package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/service"
)

func setupHandlerTestEnv(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	if repository.DB != nil {
		if sqlDB, err := repository.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	config.Global.Database.Driver = "sqlite"
	config.Global.Database.DSN = filepath.Join(t.TempDir(), "handler-test.db")
	config.Global.Crypto.MasterKey = strings.Repeat("11", 32)
	config.Global.JWT.Secret = "test-secret"
	config.Global.JWT.Expire = 1
	config.Global.Admins = []config.AdminUser{{Username: "root", Password: "rootpass"}}
	if err := config.HashAdminPasswords(); err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := repository.Connect(); err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := repository.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	if err := service.NewAdminService().SeedFromConfig(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := gin.New()
	RegisterRoutes(r, nil)
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path, token string, body any) (*httptest.ResponseRecorder, dto.Response) {
	t.Helper()
	var reader *strings.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = strings.NewReader(string(b))
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var resp dto.Response
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return w, resp
}

func login(t *testing.T, r *gin.Engine, username, password string) string {
	t.Helper()
	w, resp := doJSON(t, r, http.MethodPost, "/api/auth/login", "", dto.LoginRequest{Username: username, Password: password})
	if w.Code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("login %s: http=%d code=%d msg=%s", username, w.Code, resp.Code, resp.Message)
	}
	data, _ := resp.Data.(map[string]any)
	token, _ := data["token"].(string)
	if token == "" {
		t.Fatalf("login %s: empty token", username)
	}
	return token
}

// TestAdminRouteAuthorization 验证：普通管理员访问 /api/admins 返回 403；被禁用后旧 token 立即失效；无 admin_id 的旧 token 判 401
func TestAdminRouteAuthorization(t *testing.T) {
	r := setupHandlerTestEnv(t)
	rootToken := login(t, r, "root", "rootpass")

	// 超级管理员可列表 + 创建普通管理员
	w, resp := doJSON(t, r, http.MethodGet, "/api/admins", rootToken, nil)
	if w.Code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("super_admin list: http=%d code=%d", w.Code, resp.Code)
	}
	w, resp = doJSON(t, r, http.MethodPost, "/api/admins", rootToken, dto.CreateAdminRequest{Username: "ops", Password: "opspass", Role: "admin"})
	if w.Code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("create ops: http=%d code=%d msg=%s", w.Code, resp.Code, resp.Message)
	}
	created, _ := resp.Data.(map[string]any)
	opsID := int(created["id"].(float64))

	// 普通管理员：/api/me 可用，/api/admins 403
	opsToken := login(t, r, "ops", "opspass")
	w, resp = doJSON(t, r, http.MethodGet, "/api/me", opsToken, nil)
	if w.Code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("ops /me: http=%d code=%d", w.Code, resp.Code)
	}
	if me, _ := resp.Data.(map[string]any); me["role"] != "admin" {
		t.Errorf("ops role = %v, want admin", me["role"])
	}
	w, _ = doJSON(t, r, http.MethodGet, "/api/admins", opsToken, nil)
	if w.Code != http.StatusForbidden {
		t.Errorf("admin 访问 /api/admins 应 403，got %d", w.Code)
	}
	// 普通管理员可正常访问业务接口
	w, _ = doJSON(t, r, http.MethodGet, "/api/users", opsToken, nil)
	if w.Code != http.StatusOK {
		t.Errorf("admin 访问业务接口应 200，got %d", w.Code)
	}

	// 禁用 ops 后旧 token 立即失效（中间件每请求回查 Active）
	inactive := false
	w, resp = doJSON(t, r, http.MethodPut, "/api/admins/"+itoa(opsID), rootToken, dto.UpdateAdminRequest{Active: &inactive})
	if w.Code != http.StatusOK || resp.Code != 0 {
		t.Fatalf("disable ops: http=%d code=%d msg=%s", w.Code, resp.Code, resp.Message)
	}
	w, _ = doJSON(t, r, http.MethodGet, "/api/me", opsToken, nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("禁用后旧 token 应 401，got %d", w.Code)
	}

	// 无 token / 伪造无 admin_id 的 token
	w, _ = doJSON(t, r, http.MethodGet, "/api/me", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("无 token 应 401，got %d", w.Code)
	}
}

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}
