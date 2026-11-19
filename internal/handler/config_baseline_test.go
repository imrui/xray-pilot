package handler

import (
	"bufio"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/service"
	"github.com/imrui/xray-pilot/internal/xray"
	"github.com/imrui/xray-pilot/pkg/types"
	"golang.org/x/crypto/ssh"
)

const baselineTestPath = "/etc/xray/custom.json"

// 使用回环 SSH 会话读临时文件，只模拟远端预检与重启回执。面板侧走真实路由、租约、队列和同步事务。
func baselineSSHNode(t *testing.T) (entity.Node, string) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "remote.json")
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(private, "test")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "key")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	if err := repository.NewSettingRepository().Set(service.KeySSHKnownHostsPath, filepath.Join(dir, "known_hosts")); err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			server, channels, requests, err := ssh.NewServerConn(conn, config)
			if err != nil {
				conn.Close()
				continue
			}
			go ssh.DiscardRequests(requests)
			for incoming := range channels {
				ch, requests, err := incoming.Accept()
				if err != nil {
					continue
				}
				req := <-requests
				var args struct{ Command string }
				if req != nil && req.Type == "exec" && ssh.Unmarshal(req.Payload, &args) == nil {
					req.Reply(true, nil)
					status := baselineSSHCommand(file, args.Command, ch)
					ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
				}
				ch.Close()
			}
			server.Close()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-done })
	node := entity.Node{Name: "baseline-test", IP: "127.0.0.1", SSHPort: listener.Addr().(*net.TCPAddr).Port, SSHKeyPath: keyPath, Active: true}
	if err := repository.NewNodeRepository().Create(&node); err != nil {
		t.Fatal(err)
	}
	if err := repository.DB.Model(&entity.RoutingBinding{}).Where("scope = ? AND target_id = ?", "node", node.ID).Update("config_path", baselineTestPath).Error; err != nil {
		t.Fatal(err)
	}
	return node, file
}

func baselineSSHCommand(file, cmd string, ch ssh.Channel) uint32 {
	switch {
	case strings.HasPrefix(cmd, "test -f ") && strings.Contains(cmd, baselineTestPath):
		raw, err := os.ReadFile(file)
		if err != nil {
			return 1
		}
		ch.Write(raw)
	case strings.Contains(cmd, "PILOT:ROLLBACK_READY"):
		r := bufio.NewReader(ch)
		line, _ := r.ReadString('\n')
		var envelope struct{ Config json.RawMessage }
		if json.Unmarshal([]byte(line), &envelope) != nil || len(envelope.Config) == 0 {
			return 1
		}
		if _, err := os.Stat(file + ".fail"); err == nil {
			fmt.Fprintln(ch, "PREFLIGHT_FAILED")
			return 1
		}
		fmt.Fprintln(ch, "PILOT:READY")
		if decision, _ := r.ReadString('\n'); decision != "COMMIT\n" {
			return 1
		}
		if err := os.WriteFile(file, envelope.Config, 0600); err != nil {
			return 1
		}
		fmt.Fprintln(ch, "VERIFIED")
	case strings.HasPrefix(cmd, "systemctl show xray"):
		io.WriteString(ch, "17\n123")
	case strings.HasPrefix(cmd, "xray version"):
		io.WriteString(ch, "test")
	default:
		return 1
	}
	return 0
}

func TestBaselineInspectionAndAdoption(t *testing.T) {
	r := setupHandlerTestEnv(t)
	root := login(t, r, "root", "rootpass")
	doJSON(t, r, http.MethodPost, "/api/admins", root, dto.CreateAdminRequest{Username: "ops", Password: "opspass", Role: "admin"})
	ops := login(t, r, "ops", "opspass")
	node, file := baselineSSHNode(t)
	base := fmt.Sprintf("/api/nodes/%d/", node.ID)
	for _, action := range []string{"inspect-baseline", "adopt-baseline"} {
		w, _ := doJSON(t, r, http.MethodPost, base+action, ops, nil)
		if w.Code != 403 {
			t.Fatalf("%s returned %d", action, w.Code)
		}
	}
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	changes := func() int {
		var count int
		if err := repository.DB.Raw("SELECT total_changes()").Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		return count
	}
	inspect := func() service.BaselineInspection {
		t.Helper()
		before := changes()
		w, res := doJSON(t, r, http.MethodPost, base+"inspect-baseline", root, nil)
		if w.Code != 200 || res.Code != 0 || changes() != before {
			t.Fatalf("inspect status=%d response=%s or database changed", w.Code, res.Message)
		}
		if strings.Contains(w.Body.String(), "sensitive-test-") {
			t.Fatal("inspection exposed a credential")
		}
		var result struct{ Data service.BaselineInspection }
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result.Data
	}
	if _, res := doJSON(t, r, http.MethodPost, base+"inspect-baseline", root, nil); res.Code == 0 || !strings.Contains(res.Message, "不可读") {
		t.Fatalf("missing remote file: %+v", res)
	}
	write("invalid sensitive-test-not-json")
	if out := inspect(); out.RemoteValidJSON || out.RemoteRedacted != "<invalid json>" || out.DesiredError == "" || out.RemoteBytes != len("invalid sensitive-test-not-json") {
		t.Fatalf("invalid inspection: %+v", out)
	}
	p := entity.InboundProfile{Name: "Reality", Protocol: types.ProtocolVlessReality, Port: 443, Active: true, Settings: `{"sni":"www.microsoft.com","private_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE","short_ids":["abcd"]}`}
	if err := repository.DB.Create(&p).Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.DB.Create(&entity.NodeProfileKey{NodeID: node.ID, ProfileID: p.ID, Settings: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	remote := `{"nested":[{"privateKey":"sensitive-test-key","password":"sensitive-test-pw","secretKey":"sensitive-test-wg","id":"sensitive-test-id"}],"certificates":[{"key":["sensitive-test-pem"]}],"headers":{"Authorization":"sensitive-test-header"},"serial":12345678901234567890}`
	write(remote)
	out := inspect()
	if !strings.Contains(out.RemoteRedacted, "12345678901234567890") {
		t.Fatal("redaction changed a non-sensitive numeric value")
	}
	if !out.RemoteValidJSON || strings.Count(out.RemoteRedacted, "<masked>") != 6 || out.ConfigPath != baselineTestPath || out.ObservedHash != xray.ConfigHash(remote) || out.ObservedRawSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(remote))) || out.DesiredHash == "" || strings.Contains(out.DesiredRedacted, "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAE") || out.BaselineMatched {
		t.Fatalf("incorrect inspection: %+v", out)
	}
	req := service.AdoptBaselineRequest{ObservedRawSHA256: out.ObservedRawSHA256, Key: "test-adopt-key"}
	for _, invalid := range []service.AdoptBaselineRequest{
		{ObservedRawSHA256: strings.Repeat("z", 64), Key: req.Key},
		{ObservedRawSHA256: out.ObservedRawSHA256, Key: "short"},
		{ObservedRawSHA256: out.ObservedRawSHA256, Key: strings.Repeat("x", 101)},
	} {
		if _, res := doJSON(t, r, http.MethodPost, base+"adopt-baseline", root, invalid); res.Code != 400 {
			t.Fatalf("invalid adoption accepted: %s", res.Message)
		}
	}
	_, release, err := repository.AcquireConfigLease(node.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"inspect-baseline", "adopt-baseline"} {
		w, _ := doJSON(t, r, http.MethodPost, base+action, root, req)
		if w.Code != 409 {
			t.Fatalf("active lease %s: %d", action, w.Code)
		}
	}
	release()
	if err := repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", node.ID).Update("disk_hash", "before-conflict").Error; err != nil {
		t.Fatal(err)
	}
	write(remote + "\n")
	w, res := doJSON(t, r, http.MethodPost, base+"adopt-baseline", root, req)
	if w.Code != 409 || res.Message != service.ErrBaselineChanged.Error() {
		t.Fatalf("changed bytes: %d %s", w.Code, res.Message)
	}
	var state entity.NodeConfigState
	if err := repository.DB.First(&state, node.ID).Error; err != nil || state.DiskHash != "before-conflict" {
		t.Fatal("rejected adoption changed the disk baseline", err)
	}
	var count int64
	for _, query := range []string{"SELECT count(*) FROM config_deployments", "SELECT count(*) FROM sync_logs WHERE action = 'adopt_baseline'"} {
		if err := repository.DB.Raw(query).Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("rejected adoption created a job or audit", err)
		}
	}
	// 即便审计和基线已在事务内写入，队列失败也必须整体回滚。
	write(remote)
	if err := repository.DB.Exec("CREATE TRIGGER reject_job BEFORE INSERT ON config_deployments BEGIN SELECT RAISE(ABORT, 'queue failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	if _, res := doJSON(t, r, http.MethodPost, base+"adopt-baseline", root, req); res.Code == 0 {
		t.Fatal("queue failure reported success")
	}
	if err := repository.DB.Exec("DROP TRIGGER reject_job").Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.DB.First(&state, node.ID).Error; err != nil || state.DiskHash != "before-conflict" {
		t.Fatal("queue failure committed baseline", err)
	}
	if err := repository.DB.Model(&entity.SyncLog{}).Where("action = ?", "adopt_baseline").Count(&count).Error; err != nil || count != 0 {
		t.Fatal("queue failure committed audit", err)
	}

	for _, tc := range []struct{ name, disk, legacy, raw string }{
		{"managed", "old-disk", "old-node", remote}, {"manual", "", "", "{}"}, {"empty", "", "", ""}, {"blank-with-legacy", "", "old-node", " \r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			write(tc.raw)
			var lastSync *time.Time
			if tc.legacy != "" {
				now := time.Now()
				lastSync = &now
			}
			if err := repository.DB.Model(&node).Updates(map[string]any{"config_hash": tc.legacy, "last_sync_at": lastSync}).Error; err != nil {
				t.Fatal(err)
			}
			states := repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", node.ID)
			if err := states.Updates(map[string]any{"disk_hash": tc.disk, "last_healthy_policy": "legacy", "last_healthy_overrides": "{}"}).Error; err != nil {
				t.Fatal(err)
			}
			svc := service.NewSyncService()
			if tc.name == "blank-with-legacy" {
				if err := repository.DB.Where("node_id = ?", node.ID).Delete(&entity.NodeProfileKey{}).Error; err != nil {
					t.Fatal(err)
				}
			}
			if failed := svc.SyncNode(node.ID, "admin:root"); failed.Success || !strings.Contains(failed.Error, "DRIFT_CONFLICT") {
				t.Fatalf("ordinary sync: %+v", failed)
			}
			out := inspect()
			wantRecorded := tc.disk
			if wantRecorded == "" {
				wantRecorded = tc.legacy
			}
			if out.BaselineMatched || out.RecordedBaselineHash != wantRecorded {
				t.Fatalf("unexpected baseline: %+v", out)
			}
			req := service.AdoptBaselineRequest{ObservedRawSHA256: out.ObservedRawSHA256, Key: "adopt-test-" + tc.name}
			if err := states.Updates(map[string]any{"verified_at": time.Now(), "runtime_hash": "old-runtime", "runtime_authorization_hash": "old-auth"}).Error; err != nil {
				t.Fatal(err)
			}
			w, res := doJSON(t, r, http.MethodPost, base+"adopt-baseline", root, req)
			var result struct{ Data []entity.ConfigDeployment }
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || res.Code != 0 || len(result.Data) != 1 {
				t.Fatalf("adopt: %s %v", res.Message, err)
			}
			job := result.Data[0]
			if job.Status != "pending" || job.Actor != "admin:root" || job.DesiredHash != out.DesiredHash {
				t.Fatalf("incorrect job: %+v", job)
			}
			var adopted entity.NodeConfigState
			if err := repository.DB.First(&adopted, node.ID).Error; err != nil {
				t.Fatal(err)
			}
			blank := strings.TrimSpace(tc.raw) == ""
			if adopted.InitialBaseline != blank || adopted.RecoveryAttempts != 0 || adopted.LastError != "" || adopted.RuntimeHash != "" || adopted.RuntimeAuthorizationHash != "" || adopted.VerifiedAt != nil || adopted.LastHealthyPolicy != "legacy" || adopted.LastHealthyOverrides != "{}" {
				t.Fatal("adoption changed healthy policy or kept runtime confirmation")
			}
			if blank {
				n, err := repository.NewNodeRepository().FindByID(node.ID)
				if err != nil || n.ConfigHash != "" || adopted.DiskHash != "" || adopted.InitialBaselineHash != req.ObservedRawSHA256 {
					t.Fatal("blank file did not keep exact attestation")
				}
			} else if adopted.InitialBaselineHash != "" || adopted.DiskHash != out.ObservedHash {
				t.Fatal("incorrect adopted disk baseline")
			}
			if !inspect().BaselineMatched {
				t.Fatal("inspection and sync baseline differ")
			}
			if blank {
				if err := os.WriteFile(file+".fail", nil, 0600); err != nil {
					t.Fatal(err)
				}
				if got := svc.SyncNode(node.ID, "admin:root"); !strings.Contains(got.Error, "PREFLIGHT_FAILED") {
					t.Fatalf("preflight: %+v", got)
				}
				if err := os.Remove(file + ".fail"); err != nil {
					t.Fatal(err)
				}
				write(tc.raw + " ")
				if got := svc.SyncNode(node.ID, "admin:root"); !strings.Contains(got.Error, "DRIFT_CONFLICT") {
					t.Fatalf("different blank file accepted: %+v", got)
				}
				write(tc.raw)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			done := make(chan struct{})
			go func() { defer close(done); service.RunConfigDeployments(ctx) }()
			defer func() { cancel(); <-done }()
			for job.Status != "succeeded" && ctx.Err() == nil {
				if err := repository.DB.First(&job, job.ID).Error; err != nil {
					t.Fatal(err)
				}
				if job.Status == "failed" {
					t.Fatal(job.Error)
				}
				time.Sleep(30 * time.Millisecond)
			}
			if job.Status != "succeeded" {
				t.Fatal("adopted deployment did not finish", job.Status)
			}
			var healthy entity.NodeConfigState
			if err := repository.DB.First(&healthy, node.ID).Error; err != nil || healthy.VerifiedAt == nil || healthy.RuntimeHash != out.DesiredHash || healthy.DiskHash != out.DesiredHash || healthy.InitialBaseline {
				t.Fatal("sync did not confirm applied config", err)
			}
			w, res = doJSON(t, r, http.MethodPost, base+"adopt-baseline", root, req)
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || res.Code != 0 || len(result.Data) != 1 || result.Data[0].ID != job.ID {
				t.Fatal("idempotent replay did not return the original job")
			}
			var replay entity.NodeConfigState
			if err := repository.DB.First(&replay, node.ID).Error; err != nil || replay.RuntimeHash != healthy.RuntimeHash || replay.VerifiedAt == nil {
				t.Fatal("replay cleared a confirmed runtime")
			}
			var logs []entity.SyncLog
			if err := repository.DB.Where("action = ? AND message LIKE ?", "adopt_baseline", "%"+req.ObservedRawSHA256[:12]+"%").Find(&logs).Error; err != nil || len(logs) != 1 || logs[0].Actor != "admin:root" || !strings.Contains(logs[0].Message, baselineTestPath) {
				t.Fatal("missing or duplicate adoption audit", err)
			}
		})
	}
}
