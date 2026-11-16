package service

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

func healthTCPListener(t *testing.T) int {
	t.Helper()
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
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done })
	return listener.Addr().(*net.TCPAddr).Port
}

func TestHealthProbeInheritsSSHPort(t *testing.T) {
	setupInstallTestEnv(t)
	defaultPort, overridePort := healthTCPListener(t), healthTCPListener(t)
	svc := NewHealthService()
	if err := svc.settingSvc.BatchUpdate(map[string]string{KeySSHDefaultPort: strconv.Itoa(defaultPort)}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		port int
		want int
	}{
		{"inherit system default", 0, defaultPort},
		{"explicit node override", overridePort, overridePort},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := svc.probeNode(entity.Node{IP: "127.0.0.1", SSHPort: tc.port})
			if r.sshPort != tc.want || !r.sshOK || r.sshError != nil {
				t.Fatalf("probe port=%d ok=%v err=%v, want port=%d reachable", r.sshPort, r.sshOK, r.sshError, tc.want)
			}
		})
	}
}

func TestHealthProbeRetainsConnectionError(t *testing.T) {
	setupInstallTestEnv(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	node := entity.Node{Name: "unreachable", IP: "127.0.0.1", SSHPort: port, LastCheckOK: true}
	if err := repository.DB.Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewHealthService()
	r := svc.probeNode(node)
	var opErr *net.OpError
	if r.sshOK || !errors.As(r.sshError, &opErr) {
		t.Fatalf("want failed TCP dial with original error, got %+v", r)
	}
	svc.persist(r)
	logs, _, err := svc.logRepo.List(1, 10, actorSchedulerHealthCheck)
	if err != nil || len(logs) != 1 {
		t.Fatalf("health log: %v, err=%v", logs, err)
	}
	if !strings.Contains(logs[0].Message, r.sshError.Error()) || !strings.Contains(logs[0].Message, strconv.Itoa(port)) {
		t.Fatalf("health log lost connection details: %s", logs[0].Message)
	}
	current, err := svc.nodeRepo.FindByID(node.ID)
	if err != nil || current.LastCheckOK || current.LastCheckAt == nil {
		t.Fatalf("failed probe was not persisted: %+v, err=%v", current, err)
	}
}

type healthAlertTransport func(*http.Request) (*http.Response, error)

func (f healthAlertTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// 走实际 SQLite 状态机和健康检查的告警上报路径；飞书请求全部截获，不发真实通知。
func TestHealthAlertsCountConsecutiveChecksAndRecover(t *testing.T) {
	setupInstallTestEnv(t)
	svc := NewHealthService()
	if err := svc.settingSvc.BatchUpdate(map[string]string{
		KeyAlertEnabled: "true", KeyFeishuEnabled: "true", KeyAlertFeishuChatID: "test-chat",
		KeyFeishuAppID: "test-app", KeyFeishuAppSecret: "test-secret",
		KeyAlertFailThreshold: "3", KeyAlertRecoverThreshold: "2", KeyAlertRepeatHours: "0",
	}); err != nil {
		t.Fatal(err)
	}
	node := entity.Node{Name: "los1", IP: "127.0.0.1", SSHPort: 22}
	if err := repository.DB.Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	var cards []string
	svc.alertSvc.feishuSvc.httpClient = &http.Client{Transport: healthAlertTransport(func(req *http.Request) (*http.Response, error) {
		body := `{"code":0,"tenant_access_token":"test-token","expire":7200}`
		if req.URL.Path == "/open-apis/im/v1/messages" {
			var message struct {
				Content string `json:"content"`
			}
			if err := json.NewDecoder(req.Body).Decode(&message); err != nil {
				t.Fatal(err)
			}
			cards = append(cards, message.Content)
			body = `{"code":0}`
		} else if req.URL.Path != "/open-apis/auth/v3/tenant_access_token/internal" {
			t.Fatalf("unexpected outbound request: %s", req.URL)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	observe := func(ok bool) {
		r := nodeProbeResult{node: node, sshPort: 22, sshOK: ok}
		if !ok {
			r.sshError = errors.New("dial tcp 127.0.0.1:22: i/o timeout")
		}
		svc.reportAlerts([]nodeProbeResult{r})
	}
	// 两次失败后成功必须清零；下一次失败不能把之前的失败拼成连续 3 次。
	for _, ok := range []bool{false, false, true, false, false} {
		observe(ok)
	}
	if len(cards) != 0 {
		t.Fatalf("non-consecutive failures sent %d cards", len(cards))
	}
	state, err := svc.alertSvc.repo.Get(node.ID, entity.AlertKindNodeUnreachable)
	if err != nil || state.FailCount != 2 || state.Alerting {
		t.Fatalf("wrong pre-threshold state: %+v, err=%v", state, err)
	}
	observe(false)
	if len(cards) != 1 || !strings.Contains(cards[0], "连续失败 3 次") || !strings.Contains(cards[0], "i/o timeout") {
		t.Fatalf("third failure should send one detailed alert: %v", cards)
	}
	observe(false)
	observe(true)
	if len(cards) != 1 {
		t.Fatalf("repeated failure or first success sent an extra card: %v", cards)
	}
	observe(true)
	if len(cards) != 2 || !strings.Contains(cards[1], "已恢复") {
		t.Fatalf("second success should send recovery: %v", cards)
	}
	state, err = svc.alertSvc.repo.Get(node.ID, entity.AlertKindNodeUnreachable)
	if err != nil || state.Alerting || state.NotifiedAlerting || state.FailCount != 0 {
		t.Fatalf("wrong recovered state: %+v, err=%v", state, err)
	}
}
