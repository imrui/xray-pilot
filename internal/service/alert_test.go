package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/pkg/types"
)

var alertT0 = time.Date(2026, 10, 14, 14, 0, 0, 0, time.Local)

// step 推进一次并返回此刻是否需要发送；模拟「送达成功」时回写
func step(st *entity.NodeAlertState, ok bool, at time.Time, delivered bool) alertNotifyKind {
	advanceAlertState(st, ok, "boom", at, 3, 2)
	n := alertNotifyFor(st, at, 0)
	if n != alertNotifyNone && delivered {
		markAlertNotified(st, at)
	}
	return n
}

func TestAlertState_FiresAfterThreshold(t *testing.T) {
	st := &entity.NodeAlertState{}
	for i := 0; i < 2; i++ {
		if n := step(st, false, alertT0.Add(time.Duration(i)*time.Minute), true); n != alertNotifyNone {
			t.Fatalf("第 %d 次失败不应触发，got %v", i+1, n)
		}
	}
	if n := step(st, false, alertT0.Add(2*time.Minute), true); n != alertNotifyFiring {
		t.Fatalf("第 3 次失败应触发，got %v", n)
	}
	if !st.Since.Equal(alertT0) {
		t.Errorf("Since 应为失败串起点 %v，got %v", alertT0, st.Since)
	}
	if n := step(st, false, alertT0.Add(3*time.Minute), true); n != alertNotifyNone {
		t.Errorf("已送达后持续失败不应重复发送（repeat=0），got %v", n)
	}
}

func TestAlertState_FlappingDoesNotFire(t *testing.T) {
	st := &entity.NodeAlertState{}
	seq := []bool{false, false, true, false, false, true}
	for i, ok := range seq {
		if n := step(st, ok, alertT0.Add(time.Duration(i)*time.Minute), true); n != alertNotifyNone {
			t.Fatalf("抖动第 %d 步不应触发，got %v", i, n)
		}
	}
}

func TestAlertState_RecoverAfterThreshold(t *testing.T) {
	st := &entity.NodeAlertState{}
	for i := 0; i < 3; i++ {
		step(st, false, alertT0, true)
	}
	if n := step(st, true, alertT0, true); n != alertNotifyNone {
		t.Fatalf("1 次成功不应恢复（阈值 2），got %v", n)
	}
	// 中途再失败：仍处于告警，Since 不应被改写
	step(st, false, alertT0.Add(time.Minute), true)
	if !st.Since.Equal(alertT0) {
		t.Errorf("告警期间失败不应改写 Since")
	}
	step(st, true, alertT0, true)
	if n := step(st, true, alertT0, true); n != alertNotifyResolved {
		t.Fatalf("连续 2 次成功应恢复，got %v", n)
	}
	if st.Alerting || st.NotifiedAlerting {
		t.Errorf("恢复送达后应全部为 false")
	}
}

func TestAlertState_RetryUntilDelivered(t *testing.T) {
	st := &entity.NodeAlertState{}
	for i := 0; i < 3; i++ {
		step(st, false, alertT0, false)
	}
	// 送达失败：下一轮仍为 firing
	if n := step(st, false, alertT0, false); n != alertNotifyFiring {
		t.Fatalf("未送达应在下一轮重试，got %v", n)
	}
}

func TestAlertState_ResolvedBeforeDeliveredIsSilent(t *testing.T) {
	st := &entity.NodeAlertState{}
	for i := 0; i < 3; i++ {
		step(st, false, alertT0, false)
	}
	step(st, true, alertT0, false)
	if n := step(st, true, alertT0, false); n != alertNotifyNone {
		t.Fatalf("告警未送达即恢复，不应发孤立的恢复消息，got %v", n)
	}
}

func TestAlertState_Repeat(t *testing.T) {
	st := &entity.NodeAlertState{}
	for i := 0; i < 3; i++ {
		step(st, false, alertT0, true)
	}
	if n := alertNotifyFor(st, alertT0.Add(5*time.Hour), 6*time.Hour); n != alertNotifyNone {
		t.Errorf("未到重复间隔不应提醒，got %v", n)
	}
	if n := alertNotifyFor(st, alertT0.Add(6*time.Hour), 6*time.Hour); n != alertNotifyRepeat {
		t.Errorf("到达重复间隔应提醒，got %v", n)
	}
}

func TestAlertConfigThresholds_SyncFailedFixedOne(t *testing.T) {
	cfg := alertConfig{failThreshold: 3, recoverThreshold: 2}
	if f, r := cfg.thresholds(entity.AlertKindSyncFailed); f != 1 || r != 1 {
		t.Errorf("sync_failed 阈值应固定 1/1，got %d/%d", f, r)
	}
	if f, r := cfg.thresholds(entity.AlertKindXrayDown); f != 3 || r != 2 {
		t.Errorf("xray_down 应使用配置阈值，got %d/%d", f, r)
	}
}

func TestNodeTCPProxyPorts(t *testing.T) {
	reality := &entity.InboundProfile{Protocol: types.ProtocolVlessReality, Port: 443}
	trojan := &entity.InboundProfile{Protocol: types.ProtocolTrojan, Port: 443}
	hy2 := &entity.InboundProfile{Protocol: types.ProtocolHysteria2, Port: 8443}
	keys := []entity.NodeProfileKey{
		{Profile: reality},
		{Profile: trojan, Port: 8443}, // 节点级端口覆盖
		{Profile: hy2},                // UDP，跳过
		{Profile: reality},            // 重复端口去重
		{Profile: nil},
	}
	got := nodeTCPProxyPorts(keys)
	if len(got) != 2 || got[0] != 443 || got[1] != 8443 {
		t.Errorf("got %v, want [443 8443]", got)
	}
}

func TestAllSSHDown(t *testing.T) {
	if allSSHDown([]nodeProbeResult{{sshOK: false}}) {
		t.Error("单节点不应判定为面板级故障")
	}
	if !allSSHDown([]nodeProbeResult{{sshOK: false}, {sshOK: false}}) {
		t.Error("全部失败应判定为面板级故障")
	}
	if allSSHDown([]nodeProbeResult{{sshOK: false}, {sshOK: true}}) {
		t.Error("部分失败不应判定为面板级故障")
	}
}

// cardJSON 序列化卡片；关闭 HTML 转义，便于直接比对 <font> 标签
func cardJSON(card map[string]any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(card)
	return buf.String()
}

func TestBuildAlertCard(t *testing.T) {
	since := alertT0
	node := &entity.Node{Name: "tokyo-1", IP: "1.2.3.4", Region: "日本"}
	items := []alertItem{
		{node: node, notify: alertNotifyFiring, state: &entity.NodeAlertState{Kind: entity.AlertKindXrayDown, LastError: "TCP 端口 443 不通", Since: &since, FailCount: 3}},
		{node: nil, notify: alertNotifyResolved, state: &entity.NodeAlertState{Kind: entity.AlertKindPanelUnreachable, Since: &since}},
	}
	s := cardJSON(buildAlertCard(items, "https://panel.example.com", alertT0.Add(18*time.Minute)))
	for _, want := range []string{
		`"template":"red"`,
		"1 异常 / 1 恢复",
		"<font color='red'>[异常]</font> **tokyo-1 (1.2.3.4) · 日本**",
		"Xray 端口不通：TCP 端口 443 不通",
		"连续失败 3 次",
		"<font color='green'>[恢复]</font> **面板**",
		"持续 18 分钟",
		"https://panel.example.com/nodes",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("卡片缺少 %q\n%s", want, s)
		}
	}

	onlyResolved := cardJSON(buildAlertCard(items[1:], "", alertT0))
	if !strings.Contains(onlyResolved, `"template":"green"`) || strings.Contains(onlyResolved, "打开面板") {
		t.Errorf("仅恢复应为绿色且无面板链接时不带按钮: %s", onlyResolved)
	}
}

func TestBuildAlertCard_HeaderColor(t *testing.T) {
	since := alertT0
	mk := func(n alertNotifyKind) alertItem {
		return alertItem{notify: n, state: &entity.NodeAlertState{Kind: entity.AlertKindXrayDown, Since: &since}}
	}
	cases := []struct {
		name     string
		items    []alertItem
		template string
		title    string
	}{
		{"新异常优先红色", []alertItem{mk(alertNotifyFiring), mk(alertNotifyRepeat), mk(alertNotifyResolved)}, "red", "节点告警（1 异常 / 1 仍未恢复 / 1 恢复）"},
		{"仅重复提醒橙色", []alertItem{mk(alertNotifyRepeat), mk(alertNotifyRepeat)}, "orange", "节点告警（2 仍未恢复）"},
		{"重复提醒+恢复仍橙色", []alertItem{mk(alertNotifyRepeat), mk(alertNotifyResolved)}, "orange", "节点告警（1 仍未恢复 / 1 恢复）"},
		{"仅恢复绿色", []alertItem{mk(alertNotifyResolved)}, "green", "节点恢复（1）"},
	}
	for _, c := range cases {
		s := cardJSON(buildAlertCard(c.items, "", alertT0))
		if !strings.Contains(s, `"template":"`+c.template+`"`) || !strings.Contains(s, c.title) {
			t.Errorf("%s: want template=%s title=%q, got %s", c.name, c.template, c.title, s)
		}
	}
	if s := cardJSON(buildAlertCard([]alertItem{mk(alertNotifyRepeat)}, "", alertT0)); !strings.Contains(s, "<font color='orange'>[仍未恢复]</font>") {
		t.Errorf("仍未恢复行应为橙色标签: %s", s)
	}
}

func TestFormatAlertDuration(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Second:              "不足 1 分钟",
		18 * time.Minute:              "18 分钟",
		2*time.Hour + 5*time.Minute:   "2 小时 5 分钟",
		27*time.Hour + 30*time.Minute: "1 天 3 小时",
	}
	for d, want := range cases {
		if got := formatAlertDuration(d); got != want {
			t.Errorf("%v: got %q want %q", d, got, want)
		}
	}
}
