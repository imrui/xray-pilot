package service

import (
	"encoding/base64"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

func setupServiceTestDB(t *testing.T) {
	t.Helper()

	if repository.DB != nil {
		if sqlDB, err := repository.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}

	config.Global.Database.Driver = "sqlite"
	config.Global.Database.DSN = filepath.Join(t.TempDir(), "service-test.db")
	config.Global.Crypto.MasterKey = strings.Repeat("11", 32)

	if err := repository.Connect(); err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(func() {
		if repository.DB == nil {
			return
		}
		if sqlDB, err := repository.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
}

func TestGenerateSubscriptionIncludesNodesFromAllGroups(t *testing.T) {
	setupServiceTestDB(t)

	groupA := entity.Group{Name: "cn", Active: true}
	groupB := entity.Group{Name: "aa", Active: true}
	if err := repository.DB.Create(&groupA).Error; err != nil {
		t.Fatalf("create groupA: %v", err)
	}
	if err := repository.DB.Create(&groupB).Error; err != nil {
		t.Fatalf("create groupB: %v", err)
	}

	nodeA := entity.Node{Name: "node-a", Region: "广州", IP: "1.1.1.1", Domain: "a.example.com", Active: true, LastCheckOK: true}
	nodeB := entity.Node{Name: "node-b", Region: "香港", IP: "2.2.2.2", Domain: "b.example.com", Active: true, LastCheckOK: true}
	if err := repository.DB.Create(&nodeA).Error; err != nil {
		t.Fatalf("create nodeA: %v", err)
	}
	if err := repository.DB.Create(&nodeB).Error; err != nil {
		t.Fatalf("create nodeB: %v", err)
	}
	if err := repository.DB.Model(&groupA).Association("Nodes").Append(&nodeA); err != nil {
		t.Fatalf("append nodeA: %v", err)
	}
	if err := repository.DB.Model(&groupB).Association("Nodes").Append(&nodeB); err != nil {
		t.Fatalf("append nodeB: %v", err)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	user := entity.User{
		Username:  "tt",
		UUID:      "123e4567-e89b-12d3-a456-426614174000",
		Token:     "token-tt",
		Active:    true,
		ExpiresAt: &expiresAt,
	}
	if err := repository.DB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := repository.DB.Model(&user).Association("Groups").Replace([]entity.Group{groupA, groupB}); err != nil {
		t.Fatalf("replace user groups: %v", err)
	}

	profile := entity.InboundProfile{
		Name:     "VLESS + WS + TLS",
		Protocol: "vless-ws-tls",
		Port:     443,
		Settings: `{"host":"cdn.example.com","path":"/ws"}`,
		Active:   true,
	}
	if err := repository.DB.Create(&profile).Error; err != nil {
		t.Fatalf("create profile: %v", err)
	}
	keys := []entity.NodeProfileKey{
		{NodeID: nodeA.ID, ProfileID: profile.ID, Settings: `{}`},
		{NodeID: nodeB.ID, ProfileID: profile.ID, Settings: `{}`},
	}
	if err := repository.DB.Create(&keys).Error; err != nil {
		t.Fatalf("create node keys: %v", err)
	}

	svc := NewSubscribeService()
	encoded, err := svc.GenerateSubscription(user.Token)
	if err != nil {
		t.Fatalf("generate subscription: %v", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode subscription: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(decoded)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 subscription links, got %d: %q", len(lines), string(decoded))
	}
	if !strings.Contains(string(decoded), "a.example.com") || !strings.Contains(string(decoded), "b.example.com") {
		t.Fatalf("expected subscription to include both node domains, got %q", string(decoded))
	}
}

// TestEffectiveKeyPort 校验节点级端口覆盖优先级：key.Port > 0 时覆盖 profile.Port。
// 这是订阅 URI / Clash / sing-box 三套输出与 Xray 节点监听端口保持一致的核心保证。
func TestEffectiveKeyPort(t *testing.T) {
	profile := &entity.InboundProfile{Port: 443}

	cases := []struct {
		name    string
		profile *entity.InboundProfile
		key     *entity.NodeProfileKey
		want    int
	}{
		{"node override wins", profile, &entity.NodeProfileKey{Port: 8443}, 8443},
		{"zero port falls back to profile", profile, &entity.NodeProfileKey{Port: 0}, 443},
		{"nil key falls back to profile", profile, nil, 443},
		{"both nil returns 0", nil, nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := effectiveKeyPort(tc.profile, tc.key); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// TestSubscriptionExcludesProxyProtocols 锁死协议边界：http/socks 代理仅供运行时程序使用，
// 绝不能出现在用户订阅输出（base64 / Clash）。若未来有人给 buildURI/buildClashProxy
// 误加这两种协议的分支，此测试会失败。
func TestSubscriptionExcludesProxyProtocols(t *testing.T) {
	setupServiceTestDB(t)

	group := entity.Group{Name: "g", Active: true}
	if err := repository.DB.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	node := entity.Node{Name: "node-a", IP: "1.1.1.1", Active: true, LastCheckOK: true}
	if err := repository.DB.Create(&node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := repository.DB.Model(&group).Association("Nodes").Append(&node); err != nil {
		t.Fatalf("append node: %v", err)
	}

	user := entity.User{Username: "tt", UUID: "123e4567-e89b-12d3-a456-426614174000", Token: "token-proxy", Active: true}
	if err := repository.DB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := repository.DB.Model(&user).Association("Groups").Replace([]entity.Group{group}); err != nil {
		t.Fatalf("replace user groups: %v", err)
	}

	// 节点同时绑定 1 个用户协议 + 2 个代理协议
	profiles := []entity.InboundProfile{
		{Name: "vless", Protocol: "vless-ws-tls", Port: 443, Settings: `{"host":"cdn.example.com","path":"/ws"}`, Active: true},
		{Name: "http 代理", Protocol: "http", Port: 18080, Settings: `{"allowed_ips":["203.0.113.10"]}`, Active: true},
		{Name: "socks 代理", Protocol: "socks", Port: 11080, Settings: `{"allowed_ips":["203.0.113.10"]}`, Active: true},
	}
	if err := repository.DB.Create(&profiles).Error; err != nil {
		t.Fatalf("create profiles: %v", err)
	}
	for _, p := range profiles {
		key := entity.NodeProfileKey{NodeID: node.ID, ProfileID: p.ID, Settings: `{}`}
		if err := repository.DB.Create(&key).Error; err != nil {
			t.Fatalf("create key: %v", err)
		}
	}

	svc := NewSubscribeService()

	// base64 订阅：仅 1 条 vless 链接，不含代理端口
	encoded, err := svc.GenerateSubscription(user.Token)
	if err != nil {
		t.Fatalf("generate subscription: %v", err)
	}
	decoded, _ := base64.StdEncoding.DecodeString(encoded)
	lines := strings.Split(strings.TrimSpace(string(decoded)), "\n")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "vless://") {
		t.Fatalf("订阅应只含 1 条 vless 链接, got %q", string(decoded))
	}
	if strings.Contains(string(decoded), "18080") || strings.Contains(string(decoded), "11080") {
		t.Fatalf("订阅泄漏了代理端口: %q", string(decoded))
	}

	// Clash 输出：仅 1 个 proxy 条目
	clash, err := svc.GenerateClash(user.Token)
	if err != nil {
		t.Fatalf("generate clash: %v", err)
	}
	if got := strings.Count(clash, "- name:"); got != 1 {
		t.Fatalf("Clash 输出应只含 1 个代理条目, got %d:\n%s", got, clash)
	}
	if strings.Contains(clash, "18080") || strings.Contains(clash, "11080") {
		t.Fatalf("Clash 输出泄漏了代理端口:\n%s", clash)
	}
}
