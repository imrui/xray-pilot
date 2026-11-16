package service

import (
	"strings"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

func TestRegisterAttestedBaselineAndLegacyCompatibility(t *testing.T) {
	for _, attestation := range []string{"", strings.Repeat("AB", 32)} {
		t.Run(attestation, func(t *testing.T) {
			setupInstallTestEnv(t)
			svc := NewInstallService()
			token, err := svc.CreateToken(&dto.CreateInstallTokenRequest{Name: "bootstrap", SSHPort: 1, PanelURL: "https://panel.invalid"}, "test")
			if err != nil {
				t.Fatal(err)
			}
			tok, err := svc.AuthorizeToken(token.Token, "127.0.0.1")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.RegisterNode(tok, "127.0.0.1", &dto.RegisterNodeRequest{BootstrapConfigSHA256: "invalid"}); err == nil {
				t.Fatal("malformed attestation accepted")
			}
			if _, err := svc.AuthorizeToken(token.Token, "127.0.0.1"); err != nil {
				t.Fatalf("invalid request consumed token: %v", err)
			}
			resp, err := svc.RegisterNode(tok, "127.0.0.1", &dto.RegisterNodeRequest{BootstrapConfigSHA256: attestation})
			if err != nil {
				t.Fatal(err)
			}
			var state entity.NodeConfigState
			if err := repository.DB.First(&state, "node_id = ?", resp.NodeID).Error; err != nil {
				t.Fatal(err)
			}
			if state.InitialBaseline != (attestation != "") || state.InitialBaselineHash != strings.ToLower(attestation) {
				t.Fatal("registration baseline mismatch")
			}
		})
	}
}

func TestBootstrapBaselineSurvivesPreflightFailure(t *testing.T) {
	n, _ := routingFixture(t)
	if err := repository.EnsureConfigState(repository.DB, n.ID); err != nil {
		t.Fatal(err)
	}
	attestation := strings.Repeat("ab", 32)
	if err := repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", n.ID).Updates(map[string]any{"initial_baseline": true, "initial_baseline_hash": attestation}).Error; err != nil {
		t.Fatal(err)
	}
	saved := pushConfig
	defer func() { pushConfig = saved }()
	calls := 0
	pushConfig = func(p xray.SSHParams, raw string) *xray.SyncResult {
		calls++
		if calls == 1 {
			if !p.AllowInitial || p.InitialBaselineHash != attestation || p.OnBaseline == nil {
				t.Fatal("attestation not passed to SSH")
			}
			if err := p.OnBaseline("installer-semantic-hash"); err != nil {
				t.Fatal(err)
			}
			return &xray.SyncResult{Error: "PREFLIGHT_FAILED"}
		}
		if p.AllowInitial || p.InitialBaselineHash != "" || p.BaselineHash != "installer-semantic-hash" {
			t.Fatal("preflight retry lost baseline")
		}
		return &xray.SyncResult{Success: true, ProcessID: "17\n123"}
	}
	svc := NewSyncService()
	if svc.SyncNode(n.ID, "test").Success {
		t.Fatal("preflight failure reported success")
	}
	if r := svc.SyncNode(n.ID, "test"); !r.Success {
		t.Fatal(r.Error)
	}
}

func TestExpiryRecoveryDoesNotStopCurrentAuthorization(t *testing.T) {
	for _, mode := range []string{"confirmed", "regenerated", "missing-receipt", "stale-authorization", "changed-during-apply", "expired-lease"} {
		t.Run(mode, func(t *testing.T) {
			n, u := routingFixture(t)
			now := time.Now()
			if err := repository.DB.Model(u).Update("expires_at", now.Add(-time.Hour)).Error; err != nil {
				t.Fatal(err)
			}
			routing := NewRoutingService()
			binding, _ := routing.Binding("node", n.ID)
			binding.Overrides = `{"custom_rules":[{"type":"field","ip":["203.0.113.0/24"],"outboundTag":"direct-v4"}]}`
			if err := routing.SaveBinding(binding, binding.Version, "test", true); err != nil {
				t.Fatal(err)
			}
			savedPush, savedStop := pushConfig, stopRevokedNode
			defer func() { pushConfig = savedPush; stopRevokedNode = savedStop }()
			stops := 0
			pushes := 0
			stopRevokedNode = func(xray.SSHParams) error { stops++; return nil }
			pushConfig = func(p xray.SSHParams, raw string) *xray.SyncResult {
				pushes++
				if strings.Contains(raw, u.UUID) || strings.Contains(p.RollbackContent, u.UUID) {
					t.Fatal("revoked identity restored")
				}
				if mode == "regenerated" {
					if pushes == 1 {
						return &xray.SyncResult{Error: "AUTHORIZATION_STOPPED"}
					}
					if strings.Contains(raw, "203.0.113.0/24") {
						t.Fatal("failed routing policy was not rolled back")
					}
					return &xray.SyncResult{Success: true, ProcessID: "17\n123"}
				}
				if xray.ConfigHash(raw) == xray.ConfigHash(p.RollbackContent) {
					t.Fatal("fixture must restore a different routing policy")
				}
				r := &xray.SyncResult{Error: "ROLLED_BACK", RecoveredHash: xray.ConfigHash(p.RollbackContent), RecoveredAuthorizationHash: xssh.AuthorizationHash(p.RollbackContent), ProcessID: "17\n123"}
				switch mode {
				case "missing-receipt":
					r.RecoveredAuthorizationHash = ""
					r.ProcessID = ""
				case "stale-authorization":
					r.RecoveredAuthorizationHash = "obsolete"
				case "changed-during-apply":
					if err := repository.DB.Model(&entity.InboundProfile{}).Where("id > 0").Update("port", 8443).Error; err != nil {
						t.Fatal(err)
					}
				case "expired-lease":
					if err := repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ?", n.ID).Update("lease_until", now.Add(-time.Second)).Error; err != nil {
						t.Fatal(err)
					}
				}
				return r
			}
			r, err := NewUserService().SweepExpired(now)
			if err != nil || len(r.Swept) != 1 {
				t.Fatalf("expiry not confirmed: %+v %v", r, err)
			}
			wantStops := 1
			if mode == "confirmed" || mode == "regenerated" {
				wantStops = 0
			}
			if stops != wantStops {
				t.Fatalf("stop count %d, want %d", stops, wantStops)
			}
			var state entity.NodeConfigState
			if err := repository.DB.First(&state, "node_id = ?", n.ID).Error; err != nil {
				t.Fatal(err)
			}
			if !state.RecoveryPending {
				t.Fatal("failed routing policy marked successful")
			}
			if mode == "confirmed" && (state.RuntimeHash == state.DesiredHash || state.VerifiedAt == nil || state.Verification != "recovery_service" || state.NextRetryAt == nil || state.NextRetryAt.Before(now.Add(9*time.Minute))) {
				t.Fatalf("recovery state lost: %+v", state)
			}
			if mode == "regenerated" && (pushes != 2 || state.Verification != "recovery_verified" || state.RuntimeAuthorizationHash == "" || state.RuntimeHash == state.DesiredHash) {
				t.Fatal("regenerated recovery lost authorization receipt")
			}
		})
	}
}
