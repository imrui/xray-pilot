package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/repository"
)

func TestRelaySettingsAndUsers(t *testing.T) {
	setupInstallTestEnv(t)
	s := NewSettingService()
	if err := s.BatchUpdate(map[string]string{KeyRelayEnabled: "true"}); !errors.Is(err, ErrRelaySettings) {
		t.Fatal(err)
	}
	for _, prefix := range []string{"/sub", "ftp://host/sub", "http://user@host/sub", "https://host/sub?", "https://host/sub#x", "http://:123/sub", "http://office:65536/sub"} {
		if err := s.BatchUpdate(map[string]string{KeyRelayPrefix: prefix}); !errors.Is(err, ErrRelaySettings) {
			t.Fatal(prefix, err)
		}
	}
	base := "http://192.168.0.2:2096/sub/"
	if err := s.BatchUpdate(map[string]string{KeyRelayPrefix: " " + base + "/ "}); err != nil {
		t.Fatal(err)
	}
	if err := s.BatchUpdate(map[string]string{KeyRelayEnabled: "true"}); err != nil {
		t.Fatal(err)
	}
	users := NewUserService()
	a, err := users.Create(&dto.CreateUserRequest{Username: "one", RelaySubscriptionID: " u002abc "}, "")
	if err != nil {
		t.Fatal(err)
	}
	if a.RelaySubscriptionURL != base+"u002abc" {
		t.Fatal(a.RelaySubscriptionURL)
	}
	pageUsers, err := users.List(1, 10, "", repository.UserListFilter{Keyword: "not-on-this-page"})
	if err != nil || !pageUsers.RelayEnabled || pageUsers.RelayPrefix != base || len(pageUsers.List) != 0 {
		t.Fatal("missing relay preview prefix on empty page", pageUsers, err)
	}
	if _, err := users.Create(&dto.CreateUserRequest{Username: "two", RelaySubscriptionID: "u002abc"}, ""); !errors.Is(err, ErrRelayDuplicate) {
		t.Fatal("duplicate allowed", err)
	}
	for _, id := range []string{".", "..", "a/b", "%2f", "a?b"} {
		if _, err := users.Update(a.ID, &dto.UpdateUserRequest{RelaySubscriptionID: &id}, ""); !errors.Is(err, ErrRelayID) {
			t.Fatal(id, err)
		}
	}
	u, err := repository.NewUserRepository().FindByID(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := NewSubscribeService().GetSubscribePageData(u.Token)
	if err != nil || page.RelayURL != a.RelaySubscriptionURL || len(page.Nodes) != 0 {
		t.Fatal("page without nodes", err)
	}
	now := time.Now()
	u.ExpiresAt = &now
	if distributedRelayURL(u) != "" {
		t.Fatal("expired subscription distributed")
	}
	u.ExpiresAt = nil
	u.Active = false
	if distributedRelayURL(u) != "" {
		t.Fatal("disabled subscription distributed")
	}
	if err = s.BatchUpdate(map[string]string{KeyRelayEnabled: "false"}); err != nil {
		t.Fatal(err)
	}
	u.Active = true
	if distributedRelayURL(u) != "" || s.Get(KeyRelayPrefix) != base {
		t.Fatal("disabled settings lost")
	}
	pageUsers, err = users.List(1, 10, "", repository.UserListFilter{})
	if err != nil || pageUsers.RelayEnabled || pageUsers.RelayPrefix != "" {
		t.Fatal("disabled relay preview", pageUsers, err)
	}
	empty := ""
	if _, err := users.Update(a.ID, &dto.UpdateUserRequest{RelaySubscriptionID: &empty}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := users.Create(&dto.CreateUserRequest{Username: "two", RelaySubscriptionID: "u002abc"}, ""); err != nil {
		t.Fatal("cleared ID still reserved", err)
	}
}

type cardTransport func(*http.Request) (*http.Response, error)

func (f cardTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestRelayFeishuBatchUsesRecipientID(t *testing.T) {
	setupInstallTestEnv(t)
	prefix := "http://192.168.0.2:2096/sub/"
	if err := NewSettingService().BatchUpdate(map[string]string{KeyRelayPrefix: prefix, KeyRelayEnabled: "true", KeyFeishuEnabled: "true", KeyFeishuAppID: "test", KeyFeishuAppSecret: "test", KeyFeishuVerificationToken: "test", KeyFeishuBaseURL: "https://panel.test", KeySubscriptionBaseURL: "https://panel.test"}); err != nil {
		t.Fatal(err)
	}
	var ids []uint
	for _, name := range []string{"alpha", "bravo"} {
		u, err := NewUserService().Create(&dto.CreateUserRequest{Username: name, RelaySubscriptionID: name, FeishuOpenID: name, FeishuEnabled: true}, "")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, u.ID)
	}
	svc := NewFeishuService()
	enabled := true
	seen := map[string]int{}
	svc.httpClient.Transport = cardTransport(func(r *http.Request) (*http.Response, error) {
		reply := `{"code":0,"tenant_access_token":"test"}`
		if strings.HasSuffix(r.URL.Path, "/messages") {
			var v struct {
				ReceiveID string `json:"receive_id"`
				Content   string `json:"content"`
			}
			if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"alpha", "bravo"} {
				if strings.Contains(v.Content, prefix+name) != (enabled && name == v.ReceiveID) {
					t.Fatal("incorrect relay card", v.ReceiveID)
				}
			}
			seen[v.ReceiveID]++
			reply = `{"code":0}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(reply)), Header: http.Header{}}, nil
	})
	for i := 0; i < 2; i++ {
		result, err := svc.PushSubscriptionToUsers(ids, "admin:test")
		if err != nil || result.Sent != 2 {
			t.Fatal(result, err)
		}
		enabled = false
		if err := NewSettingService().BatchUpdate(map[string]string{KeyRelayEnabled: "false"}); err != nil {
			t.Fatal(err)
		}
	}
	if seen["alpha"] != 2 || seen["bravo"] != 2 {
		t.Fatal(seen)
	}
}
