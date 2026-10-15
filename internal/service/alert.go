package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

// actorAlert 告警发送的操作日志 actor
const actorAlert = "system:alert"

// service 实例按需 New，锁必须是包级的：
//   - alertFlushMu 串行化 Flush，防止健康检测与同步并发 Flush 重复发送同一条告警
//   - alertStateMu 串行化 Observe 的读-改-写，防止同节点并发上报（如两次同步同时结束）互相覆盖计数
var (
	alertFlushMu sync.Mutex
	alertStateMu sync.Mutex
)

// alertKindLabels 告警类型中文名（卡片展示用）
var alertKindLabels = map[string]string{
	entity.AlertKindNodeUnreachable:  "服务器不可达",
	entity.AlertKindXrayDown:         "Xray 端口不通",
	entity.AlertKindSyncFailed:       "配置同步失败",
	entity.AlertKindPanelUnreachable: "面板到全部节点不可达",
}

// AlertService 节点异常告警：状态机推进 + 合并发送飞书群卡片（v0.5.1）
type AlertService struct {
	repo       *repository.AlertStateRepository
	nodeRepo   *repository.NodeRepository
	logRepo    *repository.LogRepository
	settingSvc *SettingService
	feishuSvc  *FeishuService
}

func NewAlertService() *AlertService {
	return &AlertService{
		repo:       repository.NewAlertStateRepository(),
		nodeRepo:   repository.NewNodeRepository(),
		logRepo:    repository.NewLogRepository(),
		settingSvc: NewSettingService(),
		feishuSvc:  NewFeishuService(),
	}
}

type alertConfig struct {
	enabled          bool
	chatID           string
	failThreshold    int
	recoverThreshold int
	repeat           time.Duration
	kinds            map[string]bool
	panelURL         string
}

// loadAlertConfig 一次 GetAll 读全部相关配置，避免逐 key 查库
func (s *AlertService) loadAlertConfig() alertConfig {
	all := s.settingSvc.GetAll()
	atoi := func(key string, lo int) int {
		v, err := strconv.Atoi(strings.TrimSpace(all[key]))
		if err != nil || v < lo {
			v, _ = strconv.Atoi(settingDefaults[key])
		}
		return v
	}
	isTrue := func(key string) bool { return strings.EqualFold(strings.TrimSpace(all[key]), "true") }

	cfg := alertConfig{
		chatID:           strings.TrimSpace(all[KeyAlertFeishuChatID]),
		failThreshold:    atoi(KeyAlertFailThreshold, 1),
		recoverThreshold: atoi(KeyAlertRecoverThreshold, 1),
		repeat:           time.Duration(atoi(KeyAlertRepeatHours, 0)) * time.Hour,
		kinds:            map[string]bool{},
		panelURL:         strings.TrimRight(strings.TrimSpace(all[KeyFeishuBaseURL]), "/"),
	}
	// 告警复用飞书应用机器人：飞书集成未启用时整体不工作
	cfg.enabled = isTrue(KeyAlertEnabled) && isTrue(KeyFeishuEnabled) && cfg.chatID != ""
	for _, k := range strings.Split(all[KeyAlertKinds], ",") {
		if k = strings.TrimSpace(k); k != "" {
			cfg.kinds[k] = true
		}
	}
	return cfg
}

// thresholds 返回某类告警的触发 / 恢复阈值。sync_failed 是事件驱动（同步非周期任务），固定 1 次
func (c alertConfig) thresholds(kind string) (fail, rec int) {
	if kind == entity.AlertKindSyncFailed {
		return 1, 1
	}
	return c.failThreshold, c.recoverThreshold
}

// Enabled 告警是否处于可工作状态（总开关 + 飞书已启用 + 已选群）
func (s *AlertService) Enabled() bool {
	return s.loadAlertConfig().enabled
}

// Observe 上报一次检测结果并推进状态机。告警未启用或该类型未勾选时直接忽略，不产生 DB 写入
func (s *AlertService) Observe(nodeID uint, kind string, ok bool, detail string) {
	cfg := s.loadAlertConfig()
	if !cfg.enabled || !cfg.kinds[kind] {
		return
	}
	alertStateMu.Lock()
	defer alertStateMu.Unlock()

	st, err := s.repo.Get(nodeID, kind)
	if err != nil {
		zap.L().Warn("告警状态读取失败", zap.Uint("nodeID", nodeID), zap.String("kind", kind), zap.Error(err))
		return
	}
	// 从未失败过的节点不建行
	if st.ID == 0 && ok {
		return
	}
	fail, rec := cfg.thresholds(kind)
	advanceAlertState(st, ok, detail, time.Now(), fail, rec)
	if err := s.repo.SaveObservation(st); err != nil {
		zap.L().Warn("告警状态写入失败", zap.Uint("nodeID", nodeID), zap.String("kind", kind), zap.Error(err))
	}
}

// PruneInactive 清掉已禁用 / 已删除节点的告警状态（不发恢复）
func (s *AlertService) PruneInactive(activeNodeIDs []uint) {
	if !s.Enabled() {
		return
	}
	if err := s.repo.DeleteExceptNodes(activeNodeIDs); err != nil {
		zap.L().Warn("清理失效节点告警状态失败", zap.Error(err))
	}
}

// alertItem 卡片中的一行
type alertItem struct {
	state  *entity.NodeAlertState
	notify alertNotifyKind
	node   *entity.Node // 面板级告警为 nil
}

// Flush 收集待发送的状态翻转 / 重复提醒，合并成一张卡片发送；送达后回写 NotifiedAlerting
func (s *AlertService) Flush() {
	alertFlushMu.Lock()
	defer alertFlushMu.Unlock()

	cfg := s.loadAlertConfig()
	if !cfg.enabled {
		return
	}
	states, err := s.repo.FindPending()
	if err != nil {
		zap.L().Warn("告警状态查询失败", zap.Error(err))
		return
	}

	now := time.Now()
	items := make([]alertItem, 0)
	for i := range states {
		st := &states[i]
		if !cfg.kinds[st.Kind] {
			continue
		}
		notify := alertNotifyFor(st, now, cfg.repeat)
		if notify == alertNotifyNone {
			continue
		}
		item := alertItem{state: st, notify: notify}
		if st.NodeID != 0 {
			node, err := s.nodeRepo.FindByID(st.NodeID)
			if err != nil {
				continue // 节点已删除，级联清理会处理
			}
			item.node = node
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return
	}

	start := time.Now()
	card := buildAlertCard(items, cfg.panelURL, now)
	summary := alertSummary(items)
	if err := s.feishuSvc.SendCardToChat(cfg.chatID, card); err != nil {
		// 不回写 NotifiedAlerting，下一轮自动重试
		zap.L().Warn("告警发送失败", zap.String("summary", summary), zap.Error(err))
		s.logRepo.RecordWithActor("alert_notify", summary, actorAlert, false, err.Error(), time.Since(start).Milliseconds())
		return
	}
	for _, item := range items {
		// 只回写送达字段：发送期间 Observe 可能已推进判定（如同步失败后紧接成功），
		// 整行 Save 会把新判定覆盖回旧值，导致恢复消息永远发不出
		markAlertNotified(item.state, now)
		if err := s.repo.MarkNotified(item.state.ID, item.state.NotifiedAlerting, now); err != nil {
			zap.L().Warn("告警送达状态回写失败", zap.Uint("nodeID", item.state.NodeID), zap.Error(err))
		}
	}
	s.logRepo.RecordWithActor("alert_notify", summary, actorAlert, true, "飞书群告警已发送", time.Since(start).Milliseconds())
}

// SendTest 用已保存的配置向目标群发一张测试卡片
func (s *AlertService) SendTest(actor string) error {
	cfg := s.loadAlertConfig()
	switch {
	case !strings.EqualFold(strings.TrimSpace(s.settingSvc.Get(KeyFeishuEnabled)), "true"):
		return errors.New("飞书集成未启用")
	case cfg.chatID == "":
		return errors.New("未选择告警群")
	}
	start := time.Now()
	card := map[string]any{
		"config": map[string]any{"wide_screen_mode": true},
		"header": map[string]any{
			"template": "blue",
			"title":    map[string]any{"tag": "plain_text", "content": "Xray Pilot 告警测试"},
		},
		"elements": []any{
			map[string]any{"tag": "div", "text": map[string]any{
				"tag":     "lark_md",
				"content": fmt.Sprintf("这是一条测试消息，节点异常时将发送到本群。\n发送时间：%s", start.Format("2006-01-02 15:04:05")),
			}},
		},
	}
	err := s.feishuSvc.SendCardToChat(cfg.chatID, card)
	msg := "测试消息已发送"
	if err != nil {
		msg = err.Error()
	}
	s.logRepo.RecordWithActor("alert_test", "chat:"+maskID(cfg.chatID), actor, err == nil, msg, time.Since(start).Milliseconds())
	return err
}

// ---- 状态机（纯函数，单测覆盖） ----

type alertNotifyKind int

const (
	alertNotifyNone alertNotifyKind = iota
	alertNotifyFiring
	alertNotifyResolved
	alertNotifyRepeat
)

// advanceAlertState 推进一次检测结果：只改计数、Alerting、Since、LastError
func advanceAlertState(st *entity.NodeAlertState, ok bool, detail string, now time.Time, failThreshold, recoverThreshold int) {
	if ok {
		st.FailCount = 0
		st.OKCount++
		if st.Alerting && st.OKCount >= recoverThreshold {
			st.Alerting = false
		}
		return
	}
	st.OKCount = 0
	st.FailCount++
	st.LastError = detail
	if !st.Alerting {
		// Since 记本轮失败串的起点，告警时展示「自 xx 起」更贴近真实故障时间
		if st.FailCount == 1 {
			t := now
			st.Since = &t
		}
		if st.FailCount >= failThreshold {
			st.Alerting = true
		}
	}
}

// alertNotifyFor 判断状态是否需要发送；未送达的告警在送达前恢复时两者相等，不发孤立的「恢复」
func alertNotifyFor(st *entity.NodeAlertState, now time.Time, repeat time.Duration) alertNotifyKind {
	switch {
	case st.Alerting && !st.NotifiedAlerting:
		return alertNotifyFiring
	case !st.Alerting && st.NotifiedAlerting:
		return alertNotifyResolved
	case st.Alerting && repeat > 0 && st.LastNotifiedAt != nil && now.Sub(*st.LastNotifiedAt) >= repeat:
		return alertNotifyRepeat
	}
	return alertNotifyNone
}

func markAlertNotified(st *entity.NodeAlertState, now time.Time) {
	st.NotifiedAlerting = st.Alerting
	t := now
	st.LastNotifiedAt = &t
}

// ---- 卡片组装 ----

// alertNotifyStyle 每行标签文字与颜色：新异常红、仍未恢复橙、恢复绿
func alertNotifyStyle(n alertNotifyKind) (tag, color string) {
	switch n {
	case alertNotifyResolved:
		return "恢复", "green"
	case alertNotifyRepeat:
		return "仍未恢复", "orange"
	default:
		return "异常", "red"
	}
}

// buildAlertCard 标题栏颜色取本轮最严重的一类：有新异常红 > 仅仍未恢复橙 > 仅恢复绿
func buildAlertCard(items []alertItem, panelURL string, now time.Time) map[string]any {
	firing, repeat, resolved := 0, 0, 0
	for _, it := range items {
		switch it.notify {
		case alertNotifyResolved:
			resolved++
		case alertNotifyRepeat:
			repeat++
		default:
			firing++
		}
	}

	template, title := "green", fmt.Sprintf("Xray Pilot 节点恢复（%d）", resolved)
	if firing > 0 || repeat > 0 {
		template = "red"
		if firing == 0 {
			template = "orange"
		}
		parts := make([]string, 0, 3)
		for _, c := range []struct {
			n     int
			label string
		}{{firing, "异常"}, {repeat, "仍未恢复"}, {resolved, "恢复"}} {
			if c.n > 0 {
				parts = append(parts, fmt.Sprintf("%d %s", c.n, c.label))
			}
		}
		title = "Xray Pilot 节点告警（" + strings.Join(parts, " / ") + "）"
	}

	elements := make([]any, 0, len(items)+2)
	for _, it := range items {
		elements = append(elements, map[string]any{
			"tag":  "div",
			"text": map[string]any{"tag": "lark_md", "content": alertItemText(it, now)},
		})
	}
	if panelURL != "" {
		elements = append(elements,
			map[string]any{"tag": "hr"},
			map[string]any{"tag": "action", "actions": []any{
				map[string]any{
					"tag":  "button",
					"type": "primary",
					"text": map[string]any{"tag": "plain_text", "content": "打开面板"},
					"url":  panelURL + "/nodes",
				},
			}},
		)
	}

	return map[string]any{
		"config":   map[string]any{"wide_screen_mode": true},
		"header":   map[string]any{"template": template, "title": map[string]any{"tag": "plain_text", "content": title}},
		"elements": elements,
	}
}

func alertItemText(it alertItem, now time.Time) string {
	st := it.state
	label := alertKindLabels[st.Kind]
	if label == "" {
		label = st.Kind
	}

	tag, color := alertNotifyStyle(it.notify)

	var b strings.Builder
	fmt.Fprintf(&b, "<font color='%s'>[%s]</font> **%s**", color, tag, alertNodeLabel(it.node))
	b.WriteString("\n")
	since := "-"
	duration := ""
	if st.Since != nil {
		since = st.Since.Format("01-02 15:04")
		duration = formatAlertDuration(now.Sub(*st.Since))
	}
	switch it.notify {
	case alertNotifyResolved:
		fmt.Fprintf(&b, "%s 已恢复 · 持续 %s", label, duration)
	case alertNotifyRepeat:
		fmt.Fprintf(&b, "%s：%s\n自 %s 起 · 已持续 %s", label, st.LastError, since, duration)
	default:
		fmt.Fprintf(&b, "%s：%s\n自 %s 起 · 连续失败 %d 次", label, st.LastError, since, st.FailCount)
	}
	return b.String()
}

func alertNodeLabel(node *entity.Node) string {
	if node == nil {
		return "面板"
	}
	parts := []string{fmt.Sprintf("%s (%s)", node.Name, node.IP)}
	if node.Region != "" {
		parts = append(parts, node.Region)
	}
	if node.Owner != "" {
		parts = append(parts, node.Owner)
	}
	return strings.Join(parts, " · ")
}

func formatAlertDuration(d time.Duration) string {
	if d < time.Minute {
		return "不足 1 分钟"
	}
	minutes := int(d.Minutes())
	days, hours, mins := minutes/(24*60), (minutes/60)%24, minutes%60
	switch {
	case days > 0:
		return fmt.Sprintf("%d 天 %d 小时", days, hours)
	case hours > 0:
		return fmt.Sprintf("%d 小时 %d 分钟", hours, mins)
	default:
		return fmt.Sprintf("%d 分钟", mins)
	}
}

// alertSummary 操作日志 target：如 "tokyo-1:xray_down,面板:panel_unreachable"
func alertSummary(items []alertItem) string {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		name := "面板"
		if it.node != nil {
			name = it.node.Name
		}
		parts = append(parts, name+":"+it.state.Kind)
	}
	return strings.Join(parts, ",")
}
