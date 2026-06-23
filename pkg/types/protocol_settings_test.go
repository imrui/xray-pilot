package types

import "testing"

func TestEffectiveRealitySNI(t *testing.T) {
	cases := []struct {
		name    string
		profile *VlessRealitySettings
		key     *RealityKeyMaterial
		want    string
	}{
		{"节点覆盖优先", &VlessRealitySettings{SNI: "tpl.com"}, &RealityKeyMaterial{SNI: "node.com"}, "node.com"},
		{"节点空回退模板", &VlessRealitySettings{SNI: "tpl.com"}, &RealityKeyMaterial{}, "tpl.com"},
		{"模板节点都空用默认", &VlessRealitySettings{}, &RealityKeyMaterial{}, DefaultRealitySNI},
		{"key 为 nil 用模板", &VlessRealitySettings{SNI: "tpl.com"}, nil, "tpl.com"},
		{"全 nil 用默认", nil, nil, DefaultRealitySNI},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EffectiveRealitySNI(c.profile, c.key); got != c.want {
				t.Fatalf("EffectiveRealitySNI = %q, want %q", got, c.want)
			}
		})
	}
}

func TestEffectiveRealityFingerprint(t *testing.T) {
	cases := []struct {
		name    string
		profile *VlessRealitySettings
		key     *RealityKeyMaterial
		want    string
	}{
		{"节点覆盖优先", &VlessRealitySettings{Fingerprint: "firefox"}, &RealityKeyMaterial{Fingerprint: "safari"}, "safari"},
		{"节点空回退模板", &VlessRealitySettings{Fingerprint: "firefox"}, &RealityKeyMaterial{}, "firefox"},
		{"模板节点都空用默认", &VlessRealitySettings{}, &RealityKeyMaterial{}, DefaultRealityFingerprint},
		{"全 nil 用默认", nil, nil, DefaultRealityFingerprint},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EffectiveRealityFingerprint(c.profile, c.key); got != c.want {
				t.Fatalf("EffectiveRealityFingerprint = %q, want %q", got, c.want)
			}
		})
	}
}
