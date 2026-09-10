package entity

import (
	"testing"
	"time"
)

func TestUserEffectiveActive(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name      string
		active    bool
		expiresAt *time.Time
		expired   bool
		effective bool
	}{
		{"启用_永久", true, nil, false, true},
		{"启用_未过期", true, &future, false, true},
		{"启用_已过期", true, &past, true, false},
		{"启用_恰好到期", true, &now, true, false},
		{"禁用_未过期", false, &future, false, false},
		{"禁用_已过期", false, &past, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			u := &User{Active: c.active, ExpiresAt: c.expiresAt}
			if got := u.IsExpired(now); got != c.expired {
				t.Errorf("IsExpired = %v, want %v", got, c.expired)
			}
			if got := u.EffectiveActive(now); got != c.effective {
				t.Errorf("EffectiveActive = %v, want %v", got, c.effective)
			}
		})
	}
}
