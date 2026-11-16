package ssh

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/pkg/crypto"
)

func TestInstallerBaselineIsExactAndOneTime(t *testing.T) {
	sample := `{"inbounds":[{"port":1080,"protocol":"socks","settings":{"auth":"noauth"}}],"outbounds":[{"protocol":"freedom"}]}` + "\n"
	desired := `{"inbounds":[],"outbounds":[{"protocol":"blackhole"}]}`
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(sample)))
	for _, tc := range []struct {
		name, old, expected string
		allow, accept       bool
	}{
		{"installer-sample", sample, hash, true, true},
		{"manual-content", strings.Replace(sample, "1080", "1081", 1), hash, true, false},
		{"manual-whitespace", sample + " ", hash, true, false},
		{"missing-attestation", sample, "", true, false},
		{"unattested-empty-object", "{}", "", true, false},
		{"consumed-permission", sample, hash, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adopted := ""
			err := acceptConfigBaseline(tc.old, desired, TransactionOptions{AllowInitial: tc.allow, InitialBaselineHash: tc.expected, OnBaseline: func(s string) error { adopted = s; return nil }})
			if (err == nil) != tc.accept || (adopted != "") != tc.accept {
				t.Fatalf("baseline accepted=%v adopted=%v, want %v", err == nil, adopted != "", tc.accept)
			}
		})
	}
	if err := acceptConfigBaseline(sample, desired, TransactionOptions{BaselineHash: crypto.HashConfig(sample)}); err != nil {
		t.Fatalf("retry of established baseline failed: %v", err)
	}
	if err := acceptConfigBaseline(sample, desired, TransactionOptions{AllowInitial: true, InitialBaselineHash: hash, OnBaseline: func(string) error { return fmt.Errorf("lease lost") }}); err == nil {
		t.Fatal("baseline accepted after lease loss")
	}
}

func TestRollbackAllowsDestinationIPChangesButPreservesGuards(t *testing.T) {
	config := func(rules string) string {
		return `{"inbounds":[{"settings":{"clients":[]}}],"routing":{"rules":[{"type":"field","ip":["geoip:private"],"outboundTag":"block"}` + rules + `]}}`
	}
	old := config("")
	for _, ip := range []string{"1.1.1.1", "203.0.113.0/24", "2001:db8::/32"} {
		next := config(`,{"type":"field","ip":["` + ip + `"],"outboundTag":"direct-v4"}`)
		if got, err := rollbackCandidate(old, next, old); err != nil || got != old {
			t.Fatalf("valid destination %s rejected: %v", ip, err)
		}
		// With new credentials, choose the regenerated fallback, not the raw file.
		oldIdentity := strings.Replace(old, `"clients":[]`, `"clients":[{"id":"revoked"}]`, 1)
		if got, err := rollbackCandidate(oldIdentity, next, old); err != nil || got != old {
			t.Fatalf("current-auth fallback rejected for %s: %v", ip, err)
		}
	}
	for _, next := range []string{
		strings.Replace(old, "block", "direct", 1),
		config(`,{"inboundTag":["api"],"outboundTag":"api"}`),
		config(`,{"source":["127.0.0.1/32"],"outboundTag":"direct"}`),
	} {
		if _, err := rollbackCandidate(old, next, old); err == nil {
			t.Fatal("changed protection rules accepted an obsolete fallback")
		}
	}
}
