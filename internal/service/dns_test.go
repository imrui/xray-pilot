package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

type cloudFixture struct {
	svc               *DNSService
	node              *entity.Node
	row               dnsRecord
	status            int
	duplicates        bool
	afterPatch        func()
	requests, patches int
}

func dnsFixture(t *testing.T) *cloudFixture {
	t.Helper()
	setupInstallTestEnv(t)
	if err := NewSettingService().BatchUpdate(map[string]string{KeySSHKnownHostsPath: filepath.Join(t.TempDir(), "known_hosts")}); err != nil {
		t.Fatal(err)
	}
	f := &cloudFixture{svc: NewDNSService(), node: &entity.Node{Name: "usw2", Domain: "US.Example.test.", IP: "192.0.2.1", Active: true}, row: dnsRecord{ID: "abc", Name: "us.example.test", Type: "A", Content: "192.0.2.1"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests++
		if r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Error("missing scoped token")
		}
		if f.status != 0 {
			w.Header().Set("Retry-After", "120")
			w.WriteHeader(f.status)
			return
		}
		var result any
		switch r.Method {
		case "GET":
			if r.URL.Query().Get("name.exact") != f.row.Name {
				t.Error("missing exact name")
			}
			rows := []dnsRecord{f.row}
			if f.duplicates {
				rows = append(rows, f.row)
			}
			result = rows
		case "PATCH":
			var changes map[string]string
			if err := json.NewDecoder(r.Body).Decode(&changes); err != nil || len(changes) != 1 || changes["content"] == "" {
				t.Error("patch must only contain content", changes, err)
			}
			f.patches++
			f.row.Content = changes["content"]
			result = f.row
			if f.afterPatch != nil {
				f.afterPatch()
			}
			if f.status != 0 {
				w.WriteHeader(f.status)
				return
			}
		default:
			t.Errorf("unexpected DNS write %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": result})
	}))
	t.Cleanup(server.Close)
	f.svc.endpoint = server.URL
	if err := repository.NewNodeRepository().Create(f.node); err != nil {
		t.Fatal(err)
	}
	cfg := DNSSettingsInput{Enabled: true, Provider: "cloudflare", ZoneID: strings.Repeat("a", 32), Token: "fixture-token"}
	if err := f.svc.SaveSettings(cfg, false, "admin:test"); err != nil {
		t.Fatal(err)
	}
	f.bind(t)
	return f
}

func (f *cloudFixture) bind(t *testing.T) {
	t.Helper()
	p, err := f.svc.Preview(t.Context(), f.node.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.svc.Bind(t.Context(), f.node.ID, p, "admin:test"); err != nil {
		t.Fatal(err)
	}
}
func (f *cloudFixture) state(t *testing.T) *entity.DNSBinding {
	t.Helper()
	b, err := f.svc.Binding(f.node.ID)
	if err != nil || b == nil {
		t.Fatal("missing binding", err)
	}
	return b
}
func (f *cloudFixture) move(t *testing.T, ip string) {
	t.Helper()
	if _, err := NewNodeService().Update(f.node.ID, &dto.UpdateNodeRequest{IP: &ip}, "admin:editor"); err != nil {
		t.Fatal(err)
	}
}
func (f *cloudFixture) run(t *testing.T) {
	t.Helper()
	if err := f.svc.process(t.Context(), f.state(t).ID); err != nil {
		t.Fatal(err)
	}
}

func (f *cloudFixture) disableNode(t *testing.T) {
	t.Helper()
	if err := NewNodeService().ToggleActive(f.node.ID); err != nil {
		t.Fatal(err)
	}
	node, err := repository.NewNodeRepository().FindByID(f.node.ID)
	if err != nil || node.Active {
		t.Fatal("node was not disabled", err)
	}
}

func TestDNSDisabledNodeAutomaticSync(t *testing.T) {
	f := dnsFixture(t)
	f.disableNode(t)
	f.move(t, "192.0.2.2")
	if b := f.state(t); b.Status != entity.DNSPending || b.DesiredIP != "192.0.2.2" {
		t.Fatalf("disabled node did not queue its new address: %+v", b)
	}
	f.run(t)
	if b := f.state(t); b.Status != entity.DNSSynced || b.ConfirmedIP != "192.0.2.2" || f.row.Content != "192.0.2.2" || f.patches != 1 {
		t.Fatalf("disabled node did not update Cloudflare: %+v, patches=%d", b, f.patches)
	}
	node, err := repository.NewNodeRepository().FindByID(f.node.ID)
	if err != nil || node.Active {
		t.Fatal("DNS synchronization changed the node activation state", err)
	}
}

func TestDNSDisabledNodeManualSync(t *testing.T) {
	f := dnsFixture(t)
	f.disableNode(t)
	f.row.Content = "192.0.2.19"
	f.bind(t)
	if b := f.state(t); b.Status != entity.DNSReview || f.patches != 0 {
		t.Fatalf("binding a different address wrote without manual sync: %+v", b)
	}
	if err := f.svc.Retry(f.node.ID, "admin:manual"); err != nil {
		t.Fatal("disabled node rejected manual DNS sync", err)
	}
	f.run(t)
	if b := f.state(t); b.Status != entity.DNSSynced || b.ConfirmedIP != f.node.IP || b.Actor != "admin:manual" || f.row.Content != f.node.IP || f.patches != 1 {
		t.Fatalf("manual sync did not converge for disabled node: %+v, patches=%d", b, f.patches)
	}
}

func TestDNSDisabledNodeUnchangedAddressDoesNotWrite(t *testing.T) {
	f := dnsFixture(t)
	f.disableNode(t)
	f.bind(t)
	b := f.state(t)
	if b.Status != entity.DNSSynced || f.patches != 0 {
		t.Fatal("same-address binding unexpectedly wrote DNS")
	}
	f.move(t, f.node.IP)
	if next := f.state(t); next.Status != entity.DNSSynced || next.Version != b.Version {
		t.Fatalf("saving the same address queued an update: %+v", next)
	}
	requests := f.requests
	if err := f.svc.Retry(f.node.ID, "admin:manual"); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if b := f.state(t); b.Status != entity.DNSSynced || f.requests != requests+1 || f.patches != 0 || f.row.Content != f.node.IP {
		t.Fatalf("same-address manual sync must only read Cloudflare: %+v, requests=%d, patches=%d", b, f.requests-requests, f.patches)
	}
}

func TestDNSDisabledNodeRespectsGlobalPause(t *testing.T) {
	for _, pauseBeforeSave := range []bool{true, false} {
		name := "pause_after_queue"
		if pauseBeforeSave {
			name = "pause_before_save"
		}
		t.Run(name, func(t *testing.T) {
			f := dnsFixture(t)
			f.disableNode(t)
			if !pauseBeforeSave {
				f.move(t, "192.0.2.2")
				if f.state(t).Status != entity.DNSPending {
					t.Fatal("address change was not queued before pause")
				}
			}
			cfg := DNSSettingsInput{Provider: "cloudflare", ZoneID: strings.Repeat("a", 32)}
			if err := f.svc.SaveSettings(cfg, false, "admin:pause"); err != nil {
				t.Fatal(err)
			}
			if pauseBeforeSave {
				f.move(t, "192.0.2.2")
			}
			requests := f.requests
			if err := f.svc.Retry(f.node.ID, "admin:manual"); err == nil {
				t.Fatal("global pause allowed manual DNS sync")
			}
			f.run(t)
			if b := f.state(t); b.Status != entity.DNSReview || b.DesiredIP != "192.0.2.2" || f.requests != requests || f.patches != 0 || f.row.Content != f.node.IP {
				t.Fatalf("global pause did not block automatic DNS sync: %+v, requests=%d, patches=%d", b, f.requests-requests, f.patches)
			}
		})
	}
}

func TestDNSQueuedSyncSurvivesNodeDisable(t *testing.T) {
	f := dnsFixture(t)
	f.move(t, "192.0.2.2")
	if f.state(t).Status != entity.DNSPending {
		t.Fatal("address change was not queued")
	}
	f.disableNode(t)
	f.run(t)
	if b := f.state(t); b.Status != entity.DNSSynced || f.row.Content != "192.0.2.2" || f.patches != 1 {
		t.Fatalf("disabling node canceled queued DNS sync: %+v, patches=%d", b, f.patches)
	}
}

func TestDNSReplacementAndPause(t *testing.T) {
	f := dnsFixture(t)
	install := NewInstallService()
	issued, err := install.CreateToken(&dto.CreateInstallTokenRequest{ReplaceNodeID: &f.node.ID, PanelURL: "https://panel.test"}, "root")
	if err != nil {
		t.Fatal(err)
	}
	token, err := install.AuthorizeToken(issued.Token, "127.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	if err := install.BindTokenIP(token, "127.0.0.2"); err != nil {
		t.Fatal(err)
	}
	if _, err := install.RegisterNode(token, "127.0.0.2", &dto.RegisterNodeRequest{PublicIP: "127.0.0.2"}); err != nil {
		t.Fatal(err)
	}
	b := f.state(t)
	if b.Status != entity.DNSPending || b.DesiredIP != "127.0.0.2" || !strings.HasPrefix(b.Actor, "system:install-token:") {
		t.Fatal("replacement failed to queue DNS", b)
	}
	cfg := DNSSettingsInput{Provider: "cloudflare", ZoneID: strings.Repeat("a", 32)}
	if err := f.svc.SaveSettings(cfg, false, "admin:test"); err != nil {
		t.Fatal(err)
	}
	f.move(t, "192.0.2.12")
	cfg.Enabled = true
	if err := f.svc.SaveSettings(cfg, false, "admin:test"); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if f.state(t).Status != entity.DNSReview || f.patches != 0 {
		t.Fatal("reenable wrote without review")
	}
	if err := f.svc.Retry(f.node.ID, "admin:test"); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if f.state(t).Status != entity.DNSSynced || f.row.Content != "192.0.2.12" || f.patches != 1 {
		t.Fatal("verified record did not resume after manual sync")
	}
	cfg.ZoneID = strings.Repeat("b", 32)
	if err := f.svc.SaveSettings(cfg, false, "admin:test"); err != nil {
		t.Fatal(err)
	}
	if f.state(t).Status != entity.DNSRebind {
		t.Fatal("zone change did not suspend")
	}
}

func TestDNSContinuousMovesRecoverUnknownReceipt(t *testing.T) {
	f := dnsFixture(t)
	f.move(t, "192.0.2.2")
	// 第一笔写入已生效，回执丢失，期间管理员又保存了第三个 IP。
	f.afterPatch = func() { f.move(t, "192.0.2.3"); f.status = 504 }
	f.run(t)
	b := f.state(t)
	if b.FlightIP != "192.0.2.2" || b.DesiredIP != "192.0.2.3" || b.Status != entity.DNSPending {
		t.Fatalf("old receipt completed newer intent: %+v", b)
	}
	endpoint := f.svc.endpoint
	f.svc = NewDNSService()
	f.svc.endpoint = endpoint
	f.status = 0
	f.afterPatch = nil
	f.run(t)
	b = f.state(t)
	if f.row.Content != "192.0.2.3" || b.FlightIP != "" || b.Status != entity.DNSSynced || f.patches != 2 {
		t.Fatalf("restart recovery: %+v", b)
	}
}

func TestDNSLateSuccessfulReceipt(t *testing.T) {
	f := dnsFixture(t)
	f.move(t, "192.0.2.2")
	f.afterPatch = func() { f.move(t, "192.0.2.3") }
	f.run(t)
	b := f.state(t)
	if b.ConfirmedIP != "192.0.2.2" || b.DesiredIP != "192.0.2.3" || b.Status != entity.DNSPending {
		t.Fatalf("late success corrupted state: %+v", b)
	}
	// 管理员随后从 Cloudflare 改回原地址，已经确认的旧回执不能再覆盖它。
	f.afterPatch = nil
	f.row.Content = "192.0.2.1"
	f.run(t)
	if f.patches != 1 || f.state(t).Status != entity.DNSRebind {
		t.Fatal("external change ignored after late success")
	}
}

func TestDNSLegacyExternalReviewAfterRestart(t *testing.T) {
	f := dnsFixture(t)
	f.move(t, "192.0.2.5")
	f.row.Content = "192.0.2.11"
	b := f.state(t)
	if err := repository.DB.Model(b).Updates(map[string]any{"status": entity.DNSReview, "message": "远端当前值 192.0.2.11，请核对绑定"}).Error; err != nil {
		t.Fatal(err)
	}
	db, err := repository.DB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := repository.Connect(); err != nil {
		t.Fatal(err)
	}
	if f.state(t).Status != entity.DNSChecking || f.svc.Retry(f.node.ID, "admin:test") == nil {
		t.Fatal("legacy review allowed retry before read-only verification")
	}
	f.run(t)
	if f.state(t).Status != entity.DNSRebind || f.patches != 0 {
		t.Fatal("legacy external change was not protected")
	}
}

func TestDNSReviewAndUnbind(t *testing.T) {
	f := dnsFixture(t)
	f.row.Content = "192.0.2.19"
	f.bind(t)
	if b := f.state(t); b.Status != entity.DNSReview || f.patches != 0 {
		t.Fatal("mismatching binding wrote without request")
	}
	if err := f.svc.Retry(f.node.ID, "admin:test"); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if f.row.Content != f.node.IP {
		t.Fatal("mismatch not synchronized")
	}
	f.move(t, "192.0.2.5")
	f.row.Content = "192.0.2.11"
	f.run(t)
	conflict := f.state(t)
	if conflict.Status != entity.DNSRebind || f.patches != 1 {
		t.Fatal("external edit overwritten")
	}
	if err := f.svc.Retry(f.node.ID, "admin:test"); err == nil {
		t.Fatal("manual sync accepted an unconfirmed external change")
	}
	requests := f.requests
	f.run(t)
	if b := f.state(t); b.Status != entity.DNSRebind || b.Version != conflict.Version || f.requests != requests {
		t.Fatal("retry requeued a conflicting record", b)
	}
	f.bind(t)
	if b := f.state(t); b.ConfirmedIP != "192.0.2.11" || b.Status != entity.DNSReview || f.patches != 1 {
		t.Fatal("rebind did not capture the new baseline", b)
	}
	if err := f.svc.Retry(f.node.ID, "admin:test"); err != nil {
		t.Fatal(err)
	}
	f.run(t)
	if b := f.state(t); b.Status != entity.DNSSynced || b.ConfirmedIP != "192.0.2.5" || f.row.Content != b.ConfirmedIP || f.patches != 2 {
		t.Fatal("manual sync after rebind did not converge", b)
	}
	f.move(t, "192.0.2.6")
	f.duplicates = true
	f.run(t)
	if f.state(t).Status != entity.DNSRebind || f.patches != 2 {
		t.Fatal("duplicate record overwritten")
	}
	f.duplicates = false
	f.bind(t)
	if err := f.svc.Retry(f.node.ID, "admin:test"); err != nil {
		t.Fatal(err)
	}
	f.row.ID = "replacement"
	f.run(t)
	if f.state(t).Status != entity.DNSRebind {
		t.Fatal("new record claimed")
	}
	if err := f.svc.Unbind(f.node.ID, "admin:test"); err != nil {
		t.Fatal(err)
	}
	count := f.requests
	f.move(t, "192.0.2.7")
	if b, err := f.svc.Binding(f.node.ID); err != nil || b != nil || f.requests != count {
		t.Fatal("unbound node sent request")
	}
}

func TestDNSRetryLimitsAndBindingChanges(t *testing.T) {
	f := dnsFixture(t)
	requests := f.requests
	for range 2 {
		if err := NewNodeService().ToggleActive(f.node.ID); err != nil {
			t.Fatal(err)
		}
	}
	if f.state(t).Status != entity.DNSSynced || f.requests != requests {
		t.Fatal("toggle triggered DNS")
	}
	f.move(t, "192.0.2.7")
	f.status = 429
	for i := 1; i <= 3; i++ {
		f.run(t)
		b := f.state(t)
		if i <= 2 {
			if b.Retries != i || b.RetryAt == nil || time.Until(*b.RetryAt) < 110*time.Second {
				t.Fatalf("Retry-After discarded: %+v", b)
			}
			if err := repository.DB.Model(b).Update("retry_at", nil).Error; err != nil {
				t.Fatal(err)
			}
		} else if b.Status != entity.DNSFailed {
			t.Fatal("retry limit ignored")
		}
	}
	if err := f.svc.Retry(f.node.ID, "admin:test"); err != nil {
		t.Fatal(err)
	}
	f.status = 403
	f.run(t)
	if b := f.state(t); b.Status != entity.DNSFailed || b.Retries != 0 {
		t.Fatal("permission failure retried")
	}
	f.status = 0
	f.move(t, "2001:db8::2")
	if f.state(t).Status != entity.DNSRebind {
		t.Fatal("address family mismatch allowed")
	}
	f.move(t, "192.0.2.1")
	f.bind(t)
	domain := "new.example.test"
	if _, err := NewNodeService().Update(f.node.ID, &dto.UpdateNodeRequest{Domain: &domain}, "admin:test"); err != nil || f.state(t).Status != entity.DNSRebind {
		t.Fatal("domain changed without rebind", err)
	}
}
