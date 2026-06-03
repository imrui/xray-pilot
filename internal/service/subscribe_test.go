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
