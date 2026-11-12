package service

import (
	"errors"
	"testing"
)

func TestXrayStableVersionSettings(t *testing.T) {
	setupInstallTestEnv(t)
	svc := NewSettingService()
	if got := svc.GetAll()[KeyXrayStableVersion]; got != "26.3.27" {
		t.Fatalf("default Xray version = %q, want 26.3.27", got)
	}
	if err := svc.BatchUpdate(map[string]string{KeyXrayStableVersion: " v26.4.17 "}); err != nil {
		t.Fatalf("save version: %v", err)
	}
	// 使用新 service 实例读取，确认值经过归一化并持久化。
	if got := NewSettingService().Get(KeyXrayStableVersion); got != "26.4.17" {
		t.Fatalf("persisted version = %q, want 26.4.17", got)
	}

	for _, invalid := range []string{"", " ", "latest", "v", "26.3", "26.3.27.1", "26.3.27-beta", "26.3.27;id", "$(id)", "26.3.27\nwhoami"} {
		t.Run(invalid, func(t *testing.T) {
			err := svc.BatchUpdate(map[string]string{
				KeyXrayStableVersion: invalid,
				KeySSHDefaultPort:    "2222",
			})
			if !errors.Is(err, ErrInvalidXrayStableVersion) {
				t.Fatalf("invalid version %q: got %v", invalid, err)
			}
			if got := svc.Get(KeyXrayStableVersion); got != "26.4.17" {
				t.Errorf("invalid update changed version to %q", got)
			}
			if got := svc.Get(KeySSHDefaultPort); got != "22" {
				t.Errorf("invalid batch partially updated SSH port to %q", got)
			}
		})
	}
}
