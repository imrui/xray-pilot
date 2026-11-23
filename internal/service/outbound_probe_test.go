package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

func outboundProbeFixture(t *testing.T) *entity.Node {
	t.Helper()
	node, _ := routingFixture(t)
	const account = `"protocol":"vless","settings":{"address":"192.0.2.2","port":443,"id":"11111111-1111-4111-8111-111111111111","encryption":"none"}`
	rev := publishRouting(t, "test-outbounds", `{"schema_version":3,"ipv6_failure_policy":"strict","custom_outbounds":[
		{"tag":"outbound-los2",`+account+`},{"tag":"replacement","protocol":"freedom"},
		{"tag":"remapped",`+account+`},{"tag":"ordinary","protocol":"freedom"}
	]}`)
	routing := NewRoutingService()
	binding, err := routing.Binding("node", node.ID)
	if err != nil {
		t.Fatal(err)
	}
	binding.Mode, binding.TemplateID, binding.Revision = "template", rev.TemplateID, rev.Revision
	binding.ConfigPath = "/etc/xray/los1.json"
	binding.Overrides = `{"outbound_overrides":{"replacement":{"mode":"replace","outbound":{"tag":"replacement",` + account + `}},"remapped":{"mode":"remap","target_tag":"direct"}}}`
	if err := routing.SaveBinding(binding, binding.Version, "admin:test", true); err != nil {
		t.Fatal(err)
	}
	previous := probeRemoteOutbound
	t.Cleanup(func() { probeRemoteOutbound = previous })
	return node
}

func TestOutboundProbeUsesSavedEffectiveVLESSAndPreservesSyncState(t *testing.T) {
	node := outboundProbeFixture(t)
	const url = "https://1.0.0.1/"
	if err := NewSettingService().BatchUpdate(map[string]string{KeyXrayProbeURL: url}); err != nil {
		t.Fatal(err)
	}
	configState := func() entity.NodeConfigState {
		var state entity.NodeConfigState
		if err := repository.DB.First(&state, node.ID).Error; err != nil {
			t.Fatal(err)
		}
		return state
	}
	before := configState()
	svc := NewSyncService()
	var tags []string
	probeRemoteOutbound = func(ctx context.Context, p xray.SSHParams, tag string) (xssh.OutboundProbeMeasurements, error) {
		if _, ok := ctx.Deadline(); !ok || p.Host != node.IP || p.ConfigPath != "/etc/xray/los1.json" || p.ProbeURL != url {
			t.Fatal("missing deadline or incorrect probe parameters")
		}
		tags = append(tags, tag)
		if tag == "replacement" {
			return xssh.OutboundProbeMeasurements{}, errors.New("经此出站的 HTTPS 请求失败")
		}
		rtt := 3 * time.Millisecond
		return xssh.OutboundProbeMeasurements{HTTPSElapsed: 17 * time.Millisecond, TCPRTT: &rtt}, nil
	}
	for _, tag := range []string{"remapped", "ordinary", "unknown", "direct", ""} {
		if _, err := svc.TestOutbound(context.Background(), node.ID, tag); !errors.Is(err, ErrOutboundNotConfigured) {
			t.Fatalf("unexpected result for %q: %v", tag, err)
		}
	}
	ok, err := svc.TestOutbound(context.Background(), node.ID, "outbound-los2")
	if err != nil || !ok.OK || ok.ElapsedMs != 17 || ok.TCPRTTMs == nil || *ok.TCPRTTMs != 3 || ok.CheckedAt.IsZero() || ok.Message == "" {
		t.Fatalf("unexpected successful probe: %+v %v", ok, err)
	}
	failed, err := svc.TestOutbound(context.Background(), node.ID, "replacement")
	if err != nil || failed.OK || failed.CheckedAt.IsZero() || failed.Message != "经此出站的 HTTPS 请求失败" {
		t.Fatalf("unexpected failed probe: %+v %v", failed, err)
	}
	if !reflect.DeepEqual(tags, []string{"outbound-los2", "replacement"}) || !reflect.DeepEqual(before, configState()) {
		t.Fatalf("unexpected remote calls %v or configuration state change", tags)
	}
}

func TestOutboundProbeReportsMeasurementsIndependently(t *testing.T) {
	node := outboundProbeFixture(t)
	zero := time.Duration(0)
	subMillisecond := 350 * time.Microsecond
	httpsErr := errors.New("经此出站的 HTTPS 请求失败")
	const tcpError = "无法连接出站服务器 TCP 端口"
	for _, tc := range []struct {
		name       string
		measured   xssh.OutboundProbeMeasurements
		probeErr   error
		wantTCP    string
		wantTCPErr string
	}{
		{"TCP succeeds and HTTPS fails", xssh.OutboundProbeMeasurements{TCPRTT: &zero}, httpsErr, "0", ""},
		{"submillisecond RTT", xssh.OutboundProbeMeasurements{HTTPSElapsed: 59 * time.Millisecond, TCPRTT: &subMillisecond}, nil, "0.35", ""},
		{"TCP fails and HTTPS succeeds", xssh.OutboundProbeMeasurements{HTTPSElapsed: 59 * time.Millisecond, TCPMessage: tcpError}, nil, "", tcpError},
		{"both fail", xssh.OutboundProbeMeasurements{TCPMessage: tcpError}, httpsErr, "", tcpError},
		{"before probes start", xssh.OutboundProbeMeasurements{}, errors.New("节点探测未完成"), "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probeRemoteOutbound = func(context.Context, xray.SSHParams, string) (xssh.OutboundProbeMeasurements, error) {
				return tc.measured, tc.probeErr
			}
			result, err := NewSyncService().TestOutbound(context.Background(), node.ID, "outbound-los2")
			if err != nil {
				t.Fatal(err)
			}
			if result.OK != (tc.probeErr == nil) || result.ElapsedMs != tc.measured.HTTPSElapsed.Milliseconds() || (tc.probeErr != nil && result.Message != tc.probeErr.Error()) {
				t.Fatalf("unexpected HTTPS result: %+v", result)
			}
			payload, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(payload, &fields); err != nil {
				t.Fatal(err)
			}
			_, hasTCPErr := fields["tcp_rtt_error"]
			if string(fields["tcp_rtt_ms"]) != tc.wantTCP || hasTCPErr != (tc.wantTCPErr != "") || result.TCPRTTError != tc.wantTCPErr {
				t.Fatalf("unexpected TCP result: %s", payload)
			}
		})
	}
}

func TestOutboundProbeConcurrentRequestsAndCancellation(t *testing.T) {
	node := outboundProbeFixture(t)
	started := make(chan struct{})
	probeRemoteOutbound = func(ctx context.Context, _ xray.SSHParams, _ string) (xssh.OutboundProbeMeasurements, error) {
		close(started)
		<-ctx.Done()
		return xssh.OutboundProbeMeasurements{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		res, err := NewSyncService().TestOutbound(ctx, node.ID, "outbound-los2")
		if err == nil && (res.OK || res.Message != "测试已超时或取消") {
			err = errors.New("expected cancelled probe")
		}
		done <- err
	}()
	select {
	case <-started:
	case err := <-done:
		t.Fatalf("probe did not start: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for probe")
	}
	if _, err := NewSyncService().TestOutbound(ctx, node.ID, "replacement"); !errors.Is(err, ErrOutboundProbeBusy) {
		t.Errorf("second request on same node: %v", err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	probeRemoteOutbound = func(context.Context, xray.SSHParams, string) (xssh.OutboundProbeMeasurements, error) {
		return xssh.OutboundProbeMeasurements{}, nil
	}
	if res, err := NewSyncService().TestOutbound(context.Background(), node.ID, "outbound-los2"); err != nil || !res.OK {
		t.Fatalf("node still busy after cancellation: %+v %v", res, err)
	}
}

func TestOutboundProbeSSHErrorHidesKeyPath(t *testing.T) {
	node := outboundProbeFixture(t)
	// 使用不存在的私钥，确保 SSH 建连前可确定地失败。
	keyPath := filepath.Join(t.TempDir(), "private-key-for-los1")
	if err := repository.DB.Model(node).Update("ssh_key_path", keyPath).Error; err != nil {
		t.Fatal(err)
	}
	res, err := NewSyncService().TestOutbound(context.Background(), node.ID, "outbound-los2")
	if err != nil || res.OK || res.CheckedAt.IsZero() || res.Message != "无法通过 SSH 连接节点，请检查节点连接参数" {
		t.Fatalf("unexpected SSH failure: %+v %v", res, err)
	}
}
