package entity

import (
	"net/netip"
	"strings"
	"time"

	"golang.org/x/net/idna"
)

type DNSConfig struct {
	ID          uint   `gorm:"primaryKey;autoIncrement:false" json:"-"`
	Enabled     bool   `json:"enabled"`
	Provider    string `json:"provider"`
	ZoneID      string `json:"zone_id"`
	TokenCipher string `json:"-"`
}

const (
	DNSChecking = "checking"
	DNSPending  = "pending"
	DNSSynced   = "synced"
	DNSReview   = "review"
	DNSRebind   = "rebind"
	DNSFailed   = "failed"
)

type DNSBinding struct {
	ID            uint       `json:"-"`
	NodeID        uint       `gorm:"uniqueIndex;not null" json:"node_id"`
	ZoneID        string     `gorm:"uniqueIndex:idx_dns_record" json:"zone_id"`
	RecordID      string     `gorm:"uniqueIndex:idx_dns_record" json:"record_id"`
	Domain        string     `json:"domain"`
	Type          string     `json:"type"`
	ConfirmedIP   string     `json:"confirmed_ip"`
	DesiredIP     string     `json:"desired_ip"`
	Version       uint64     `json:"-"`
	FlightIP      string     `json:"-"`
	FlightVersion uint64     `json:"-"`
	Status        string     `gorm:"index" json:"status"`
	Message       string     `json:"message"`
	Actor         string     `json:"-"`
	Retries       int        `json:"-"`
	RetryAt       *time.Time `json:"retry_at,omitempty"`
	LeaseUntil    *time.Time `json:"-"`
}

func DNSName(name string) string {
	ascii, err := idna.Lookup.ToASCII(strings.TrimSuffix(strings.TrimSpace(name), "."))
	if err != nil {
		return ""
	}
	return strings.ToLower(ascii)
}

func DNSAddress(ip string) (string, string) {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil || addr.Zone() != "" {
		return "", ""
	}
	addr = addr.Unmap()
	if addr.Is4() {
		return addr.String(), "A"
	}
	return addr.String(), "AAAA"
}
