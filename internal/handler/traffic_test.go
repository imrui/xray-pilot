package handler

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestTrafficAnalyticsAPI(t *testing.T) {
	r := setupHandlerTestEnv(t)
	token := login(t, r, "root", "rootpass")
	end := time.Now().UTC()
	params := url.Values{"start": {end.Add(-time.Hour).Format(time.RFC3339)}, "end": {end.Format(time.RFC3339)}, "dimension": {"node"}}
	path := "/api/traffic/analytics?" + params.Encode()
	w, _ := doJSON(t, r, http.MethodGet, path, "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized http=%d", w.Code)
	}
	w, resp := doJSON(t, r, http.MethodGet, path, token, nil)
	if w.Code != 200 || resp.Code != 0 {
		t.Fatalf("analytics: http=%d %+v", w.Code, resp)
	}
	for key, value := range map[string]string{"start": "invalid", "end": "2026-01-01", "dimension": "oops", "node_id": "-1", "user_id": "x"} {
		p := url.Values{}
		for k, v := range params {
			p[k] = v
		}
		p.Set(key, value)
		_, resp := doJSON(t, r, http.MethodGet, "/api/traffic/analytics?"+p.Encode(), token, nil)
		if resp.Code != 400 {
			t.Errorf("invalid %s accepted: %+v", key, resp)
		}
	}
}
