package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/config"
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/repository"
)

// setupInstallTestEnv 复用 backup_test 的连接逻辑，但把 SSH 私钥/公钥也准备好。
// 私钥路径通过 SettingService 写入（与生产链路一致），不依赖 config.Global。
func setupInstallTestEnv(t *testing.T) (privPath string) {
	t.Helper()
	if repository.DB != nil {
		if sqlDB, err := repository.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	tmp := t.TempDir()
	config.Global.Database.Driver = "sqlite"
	config.Global.Database.DSN = filepath.Join(tmp, "test.db")
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

	// 在临时目录写一对假的 SSH 密钥；公钥内容随意（service 只关心非空）
	keyDir := filepath.Join(tmp, "ssh")
	if err := os.MkdirAll(keyDir, 0o700); err != nil {
		t.Fatalf("mkdir ssh dir: %v", err)
	}
	privPath = filepath.Join(keyDir, "id_ed25519")
	if err := os.WriteFile(privPath, []byte("fake-private"), 0o600); err != nil {
		t.Fatalf("write priv: %v", err)
	}
	if err := os.WriteFile(privPath+".pub", []byte("ssh-ed25519 AAAA... panel@xray-pilot"), 0o644); err != nil {
		t.Fatalf("write pub: %v", err)
	}
	if err := NewSettingService().BatchUpdate(map[string]string{
		KeySSHDefaultKeyPath: privPath,
	}); err != nil {
		t.Fatalf("seed ssh key path: %v", err)
	}
	return privPath
}

func TestCreateToken_RequiresPanelPubkey(t *testing.T) {
	setupInstallTestEnv(t)
	// 故意把 setting 改成不存在路径，复现"用户在系统设置页填了错路径"的场景
	if err := NewSettingService().BatchUpdate(map[string]string{
		KeySSHDefaultKeyPath: "/nonexistent/key",
	}); err != nil {
		t.Fatalf("set bad path: %v", err)
	}

	svc := NewInstallService()
	_, err := svc.CreateToken(&dto.CreateInstallTokenRequest{
		Name:     "test-node",
		PanelURL: "https://panel.example",
	}, "admin")
	if !errors.Is(err, ErrPanelSSHKeyMissing) {
		t.Fatalf("expected ErrPanelSSHKeyMissing, got %v", err)
	}
}

// 复现用户反馈：私钥存在但 .pub 不存在，install 应当报 ErrPanelSSHKeyMissing
func TestCreateToken_RequiresPubkeyFile(t *testing.T) {
	privPath := setupInstallTestEnv(t)
	// 删除 .pub，保留私钥
	if err := os.Remove(privPath + ".pub"); err != nil {
		t.Fatalf("rm pub: %v", err)
	}

	svc := NewInstallService()
	_, err := svc.CreateToken(&dto.CreateInstallTokenRequest{
		Name:     "test-node-pub-missing",
		PanelURL: "https://panel.example",
	}, "admin")
	if !errors.Is(err, ErrPanelSSHKeyMissing) {
		t.Fatalf("expected ErrPanelSSHKeyMissing when .pub missing, got %v", err)
	}
}

func TestCreateToken_RejectsDuplicateName(t *testing.T) {
	setupInstallTestEnv(t)
	svc := NewInstallService()

	// 先创建一个节点（直接走 NodeService.Create 模拟既有节点）
	_, err := NewNodeService().Create(&dto.CreateNodeRequest{
		Name: "exists",
		IP:   "1.1.1.1",
	})
	if err != nil {
		t.Fatalf("seed node: %v", err)
	}

	_, err = svc.CreateToken(&dto.CreateInstallTokenRequest{
		Name:     "exists",
		PanelURL: "https://panel.example",
	}, "admin")
	if !errors.Is(err, ErrInstallNodeAlreadyExist) {
		t.Fatalf("expected ErrInstallNodeAlreadyExist, got %v", err)
	}
}

func TestAuthorizeToken_OneShotAfterRegister(t *testing.T) {
	setupInstallTestEnv(t)
	svc := NewInstallService()

	resp, err := svc.CreateToken(&dto.CreateInstallTokenRequest{
		Name:     "n1",
		PanelURL: "https://panel.example",
	}, "admin")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// 首次 authorize + register
	tok, err := svc.AuthorizeToken(resp.Token, "1.2.3.4")
	if err != nil {
		t.Fatalf("first authorize: %v", err)
	}
	if err := svc.BindTokenIP(tok, "1.2.3.4"); err != nil {
		t.Fatalf("bind ip: %v", err)
	}
	if _, err := svc.RegisterNode(tok, "1.2.3.4", &dto.RegisterNodeRequest{PublicIP: "9.9.9.9"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	// 第二次 authorize 应当返回 ErrInstallTokenUsed
	if _, err := svc.AuthorizeToken(resp.Token, "1.2.3.4"); !errors.Is(err, ErrInstallTokenUsed) {
		t.Fatalf("expected ErrInstallTokenUsed on reuse, got %v", err)
	}
}

func TestAuthorizeToken_IPBinding(t *testing.T) {
	setupInstallTestEnv(t)
	svc := NewInstallService()
	resp, err := svc.CreateToken(&dto.CreateInstallTokenRequest{
		Name:     "n2",
		PanelURL: "https://panel.example",
	}, "admin")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	tok, err := svc.AuthorizeToken(resp.Token, "1.1.1.1")
	if err != nil {
		t.Fatalf("first authorize: %v", err)
	}
	if err := svc.BindTokenIP(tok, "1.1.1.1"); err != nil {
		t.Fatalf("bind: %v", err)
	}

	// 异地访问必须被拒
	if _, err := svc.AuthorizeToken(resp.Token, "8.8.8.8"); !errors.Is(err, ErrInstallTokenIPMismatch) {
		t.Fatalf("expected ErrInstallTokenIPMismatch, got %v", err)
	}
	// 原 IP 仍可继续访问（直至 register 后被标 used）
	if _, err := svc.AuthorizeToken(resp.Token, "1.1.1.1"); err != nil {
		t.Fatalf("same IP retry should pass, got %v", err)
	}
}

func TestAuthorizeToken_Expired(t *testing.T) {
	setupInstallTestEnv(t)
	svc := NewInstallService()

	resp, err := svc.CreateToken(&dto.CreateInstallTokenRequest{
		Name:       "n3",
		PanelURL:   "https://panel.example",
		TTLSeconds: 1, // 立即过期
	}, "admin")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// 手动把 expires_at 推到过去，避免 sleep
	repository.DB.Exec("UPDATE node_install_tokens SET expires_at = ? WHERE token = ?",
		time.Now().Add(-time.Hour), resp.Token)

	if _, err := svc.AuthorizeToken(resp.Token, "1.1.1.1"); !errors.Is(err, ErrInstallTokenExpired) {
		t.Fatalf("expected ErrInstallTokenExpired, got %v", err)
	}
}

func TestCleanupExpired_RemovesUnusedExpiredOnly(t *testing.T) {
	setupInstallTestEnv(t)
	svc := NewInstallService()

	// 一个过期未使用
	r1, _ := svc.CreateToken(&dto.CreateInstallTokenRequest{Name: "a", PanelURL: "x"}, "admin")
	repository.DB.Exec("UPDATE node_install_tokens SET expires_at = ? WHERE token = ?",
		time.Now().Add(-time.Hour), r1.Token)

	// 一个过期但已使用（应保留）
	r2, _ := svc.CreateToken(&dto.CreateInstallTokenRequest{Name: "b", PanelURL: "x"}, "admin")
	now := time.Now().Add(-time.Hour)
	repository.DB.Exec("UPDATE node_install_tokens SET expires_at = ?, used_at = ? WHERE token = ?",
		now, now, r2.Token)

	// 一个未过期未使用（应保留）
	_, _ = svc.CreateToken(&dto.CreateInstallTokenRequest{Name: "c", PanelURL: "x"}, "admin")

	deleted, err := svc.CleanupExpired()
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}
}

func TestBuildInstallCurlCommand_FormatsPanelAndToken(t *testing.T) {
	cmd := buildInstallCurlCommand("https://panel.example", "abc123")
	if !strings.Contains(cmd, "PANEL_URL=https://panel.example") {
		t.Errorf("missing PANEL_URL in cmd: %s", cmd)
	}
	if !strings.Contains(cmd, "INSTALL_TOKEN=abc123") {
		t.Errorf("missing INSTALL_TOKEN in cmd: %s", cmd)
	}
}
