package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

// TestGenerateSingboxVlessReality 验证 VLESS+Reality 节点能正确渲染为 sing-box outbound
func TestGenerateSingboxVlessReality(t *testing.T) {
	setupServiceTestDB(t)

	group := entity.Group{Name: "g1", Active: true}
	if err := repository.DB.Create(&group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	node := entity.Node{Name: "node-tokyo", Region: "JP", IP: "1.2.3.4", Domain: "tokyo.example.com", Active: true, LastCheckOK: true}
	if err := repository.DB.Create(&node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := repository.DB.Model(&group).Association("Nodes").Append(&node); err != nil {
		t.Fatalf("append node: %v", err)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	user := entity.User{
		Username:  "alice",
		UUID:      "11111111-2222-3333-4444-555555555555",
		Token:     "singbox-token-1",
		Active:    true,
		ExpiresAt: &expiresAt,
	}
	if err := repository.DB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := repository.DB.Model(&user).Association("Groups").Replace([]entity.Group{group}); err != nil {
		t.Fatalf("attach group: %v", err)
	}

	profile := entity.InboundProfile{
		Name:     "VLESS Reality",
		Protocol: "vless-reality",
		Port:     443,
		Settings: `{"sni":"www.microsoft.com","fingerprint":"chrome"}`,
		Active:   true,
	}
	if err := repository.DB.Create(&profile).Error; err != nil {
		t.Fatalf("create profile: %v", err)
	}
	key := entity.NodeProfileKey{
		NodeID:    node.ID,
		ProfileID: profile.ID,
		Settings:  `{"public_key":"PUBKEY_FOR_TEST","short_ids":["abcd1234"]}`,
	}
	if err := repository.DB.Create(&key).Error; err != nil {
		t.Fatalf("create key: %v", err)
	}

	svc := NewSubscribeService()
	out, err := svc.GenerateSingbox(user.Token)
	if err != nil {
		t.Fatalf("generate singbox: %v", err)
	}

	// 1. JSON 必须可解析
	var cfg map[string]any
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}

	// 2. 必须包含 outbounds 数组，且含节点 outbound + selector + urltest + direct/block/dns
	outbounds, ok := cfg["outbounds"].([]any)
	if !ok || len(outbounds) == 0 {
		t.Fatalf("outbounds missing or empty")
	}

	hasVless, hasSelector, hasURLTest, hasDirect := false, false, false, false
	for _, o := range outbounds {
		ob, _ := o.(map[string]any)
		switch ob["type"] {
		case "vless":
			hasVless = true
			if ob["server"] != "tokyo.example.com" {
				t.Errorf("expected server=tokyo.example.com, got %v", ob["server"])
			}
			if ob["uuid"] != user.UUID {
				t.Errorf("expected uuid=%s, got %v", user.UUID, ob["uuid"])
			}
			if ob["flow"] != "xtls-rprx-vision" {
				t.Errorf("expected flow=xtls-rprx-vision, got %v", ob["flow"])
			}
			tls, _ := ob["tls"].(map[string]any)
			reality, _ := tls["reality"].(map[string]any)
			if reality["public_key"] != "PUBKEY_FOR_TEST" {
				t.Errorf("expected public_key=PUBKEY_FOR_TEST, got %v", reality["public_key"])
			}
			if reality["short_id"] != "abcd1234" {
				t.Errorf("expected short_id=abcd1234, got %v", reality["short_id"])
			}
		case "selector":
			hasSelector = true
		case "urltest":
			hasURLTest = true
		case "direct":
			hasDirect = true
		}
	}
	if !hasVless || !hasSelector || !hasURLTest || !hasDirect {
		t.Fatalf("missing required outbound types: vless=%v selector=%v urltest=%v direct=%v",
			hasVless, hasSelector, hasURLTest, hasDirect)
	}

	// 3. route.final 应指向 select
	route, _ := cfg["route"].(map[string]any)
	if route["final"] != "select" {
		t.Errorf("expected route.final=select, got %v", route["final"])
	}
}

// TestGenerateSingboxRejectsExpired 验证过期用户被拒绝
func TestGenerateSingboxRejectsExpired(t *testing.T) {
	setupServiceTestDB(t)

	expiresAt := time.Now().Add(-1 * time.Hour)
	user := entity.User{
		Username:  "expired",
		UUID:      "00000000-0000-0000-0000-000000000000",
		Token:     "expired-token",
		Active:    true,
		ExpiresAt: &expiresAt,
	}
	if err := repository.DB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	svc := NewSubscribeService()
	if _, err := svc.GenerateSingbox(user.Token); err == nil {
		t.Fatalf("expected expired error, got nil")
	} else if !strings.Contains(err.Error(), "过期") {
		t.Fatalf("expected expiry message, got %v", err)
	}
}

// TestGenerateSingboxEmptyGroups 验证无分组用户返回最小骨架（不报错）
func TestGenerateSingboxEmptyGroups(t *testing.T) {
	setupServiceTestDB(t)

	expiresAt := time.Now().Add(24 * time.Hour)
	user := entity.User{
		Username:  "nogroup",
		UUID:      "22222222-2222-2222-2222-222222222222",
		Token:     "nogroup-token",
		Active:    true,
		ExpiresAt: &expiresAt,
	}
	if err := repository.DB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	svc := NewSubscribeService()
	out, err := svc.GenerateSingbox(user.Token)
	if err != nil {
		t.Fatalf("generate singbox: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(out), &cfg); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	// 应该有 direct/block/dns 三个兜底 outbound
	outbounds, _ := cfg["outbounds"].([]any)
	if len(outbounds) == 0 {
		t.Fatalf("expected fallback outbounds, got empty")
	}
}

// TestGenerateSingboxRejectsInactive 验证停用用户被拒绝
//
// 注意：entity.User.Active 含 `gorm:"default:true"`，Create 时 false 会被识别为零值
// 而走默认值；故先 Create 再 Update 强制写入 false
func TestGenerateSingboxRejectsInactive(t *testing.T) {
	setupServiceTestDB(t)

	user := entity.User{
		Username: "disabled",
		UUID:     "33333333-3333-3333-3333-333333333333",
		Token:    "disabled-token",
		Active:   true,
	}
	if err := repository.DB.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := repository.DB.Model(&user).Update("active", false).Error; err != nil {
		t.Fatalf("disable user: %v", err)
	}

	svc := NewSubscribeService()
	if _, err := svc.GenerateSingbox(user.Token); err == nil {
		t.Fatalf("expected disabled error, got nil")
	}
}
