package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

var ErrNodeSyncBusy = errors.New("节点正在执行同步任务，请稍后再试")

type PreflightDraftRequest struct {
	Policy           json.RawMessage  `json:"policy"`
	UseNodeOverrides bool             `json:"use_node_overrides"`
	CustomOverrides  *json.RawMessage `json:"custom_overrides"`
}

type PreflightDraftResult struct {
	Valid          bool     `json:"valid"`
	XrayVersion    string   `json:"xray_version"`
	RequiresIPv6   bool     `json:"requires_ipv6"`
	ElapsedMs      int64    `json:"elapsed_ms"`
	Warnings       []string `json:"warnings"`
	RedactedConfig string   `json:"redacted_config"`
	Error          string   `json:"error"`
}

var remoteDraftPreflight = func(p xray.SSHParams, content string, requiresIPv6 bool) (string, error) {
	c, err := xssh.Connect(xssh.Config{
		Host:           p.Host,
		Port:           p.Port,
		User:           p.User,
		KeyPath:        p.KeyPath,
		KnownHostsPath: p.KnownHostsPath,
	})
	if err != nil {
		return "", fmt.Errorf("无法连接目标节点")
	}
	defer c.Close()
	var receipt xssh.ValidateReceipt
	err = c.ApplyXrayConfig(content, p.ConfigPath, false, true, xssh.TransactionOptions{
		ProbeURL:       p.ProbeURL,
		RequiresIPv6:   requiresIPv6,
		ValidateResult: &receipt,
	})
	if err != nil {
		if receipt.Detail != "" {
			return receipt.XrayVersion, fmt.Errorf("%w: %s", err, receipt.Detail)
		}
		return receipt.XrayVersion, err
	}
	return receipt.XrayVersion, nil
}

func hasActiveConfigLease(nodeID uint) bool {
	var state entity.NodeConfigState
	if repository.DB.First(&state, "node_id = ?", nodeID).Error != nil {
		return false
	}
	return state.LeaseUntil != nil && state.LeaseUntil.After(time.Now())
}

func unwrapCustomOverrides(raw *json.RawMessage) (string, bool, error) {
	if raw == nil {
		return "", false, nil
	}
	trimmed := bytes.TrimSpace(*raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return "", false, nil
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return "", true, fmt.Errorf("custom_overrides 格式无效")
		}
		return s, true, nil
	}
	return string(trimmed), true, nil
}

// PreflightDraft 在纯内存中合成未发布草稿与目标节点配置，并在远端节点执行只读 xray -test 预检。
// 零库表写操作、礼让活跃同步租约、不重启线上服务、不触发任何同步失败告警。
func (s *SyncService) PreflightDraft(nodeID uint, req PreflightDraftRequest) (*PreflightDraftResult, error) {
	start := time.Now()
	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return nil, fmt.Errorf("节点不存在")
	}
	if len(bytes.TrimSpace(req.Policy)) == 0 {
		return nil, fmt.Errorf("请提供待预检的路由策略草稿")
	}

	out := &PreflightDraftResult{Warnings: []string{}}

	// 1. 纯内存静态校验与覆盖合并（不写库）
	baseRaw := string(req.Policy)
	basePolicy, err := xray.DecodeRoutingPolicy(baseRaw)
	if err != nil {
		out.ElapsedMs = time.Since(start).Milliseconds()
		out.Error = err.Error()
		return out, nil
	}

	effectivePolicy := basePolicy
	customOv, hasCustomOv, ovErr := unwrapCustomOverrides(req.CustomOverrides)
	if ovErr != nil {
		out.ElapsedMs = time.Since(start).Milliseconds()
		out.Error = ovErr.Error()
		return out, nil
	}
	if hasCustomOv {
		if err := xray.ValidateRoutingOverrides(customOv); err != nil {
			out.ElapsedMs = time.Since(start).Milliseconds()
			out.Error = err.Error()
			return out, nil
		}
		merged, mergeErr := xray.MergeRoutingPolicy(baseRaw, customOv)
		if mergeErr != nil {
			out.ElapsedMs = time.Since(start).Milliseconds()
			out.Error = mergeErr.Error()
			return out, nil
		}
		effectivePolicy = merged
	} else if req.UseNodeOverrides {
		if b, bindErr := NewRoutingService().Binding("node", nodeID); bindErr == nil && b.Mode != "legacy" && b.Overrides != "" && b.Overrides != "{}" {
			merged, mergeErr := xray.MergeRoutingPolicy(baseRaw, b.Overrides)
			if mergeErr == nil {
				effectivePolicy = merged
			} else {
				out.Warnings = append(out.Warnings, fmt.Sprintf("目标节点现有覆盖项与当前草稿不兼容（%v），本次仅按草稿本身预检", mergeErr))
			}
		}
	}

	// 2. 内存合成节点完整配置（加密草稿如无原密文亦可直接解密渲染）
	profileKeys, err := s.profileRepo.FindActiveKeysForNode(node.ID)
	if err != nil {
		return nil, fmt.Errorf("查询节点协议配置失败: %w", err)
	}
	if len(profileKeys) == 0 {
		out.Warnings = append(out.Warnings, "该节点尚未绑定激活的入站协议，本次仅校验路由与出站结构")
	}
	users, err := s.userRepo.FindActiveUsersByNodeID(node.ID)
	if err != nil {
		return nil, fmt.Errorf("查询节点用户失败: %w", err)
	}
	logLevel := node.LogLevel
	if logLevel == "" {
		logLevel = s.settingSvc.Get(KeyXrayLogLevel)
	}
	logCfg := xray.LogConfig{
		Access: s.settingSvc.Get(KeyXrayLogAccess),
		Error:  s.settingSvc.Get(KeyXrayLogError),
		Level:  logLevel,
	}
	baseConfig, genWarnings, err := xray.GenerateConfig(node, profileKeys, users, logCfg)
	if len(genWarnings) > 0 {
		out.Warnings = append(out.Warnings, genWarnings...)
	}
	if err != nil {
		out.ElapsedMs = time.Since(start).Milliseconds()
		out.Error = err.Error()
		return out, nil
	}

	renderOpts := xray.RenderOptions{
		DefaultProbeURL: s.settingSvc.Get(KeyXrayProbeURL),
	}
	rendered, err := xray.ApplyRoutingPolicyWithOptions(baseConfig, effectivePolicy, renderOpts)
	if err != nil {
		out.ElapsedMs = time.Since(start).Milliseconds()
		out.Error = err.Error()
		return out, nil
	}
	out.Warnings = append(out.Warnings, xray.DiagnoseRenderedDNSSelfLoop(baseConfig, effectivePolicy, renderOpts)...)
	out.RequiresIPv6 = effectivePolicy.RequiresIPv6WithOptions(renderOpts)
	out.RedactedConfig = redactConfig(rendered)

	// 3. 礼让线上活跃部署租约，复用进程内 lockNode 串行执行远端只读 xray -test
	if hasActiveConfigLease(nodeID) {
		return nil, ErrNodeSyncBusy
	}
	unlock := lockNode(nodeID)
	defer unlock()
	if hasActiveConfigLease(nodeID) {
		return nil, ErrNodeSyncBusy
	}

	sshParams := s.sshParams(node)
	xrayVer, remoteErr := remoteDraftPreflight(sshParams, rendered, out.RequiresIPv6)
	out.ElapsedMs = time.Since(start).Milliseconds()
	out.XrayVersion = xrayVer
	if remoteErr != nil {
		out.Valid = false
		out.Error = scrubConfigSecretsFromMessage(remoteErr.Error(), rendered)
		return out, nil
	}
	out.Valid = true
	return out, nil
}

func (s *SyncService) PreflightNode(nodeID uint) error {
	defer lockNode(nodeID)()
	node, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return err
	}
	content, _, err := s.buildConfig(node)
	if err != nil {
		return err
	}
	snapshot, err := NewRoutingService().Effective(nodeID)
	if err != nil {
		return err
	}
	p := s.sshParams(node)
	c, err := xssh.Connect(xssh.Config{Host: p.Host, Port: p.Port, User: p.User, KeyPath: p.KeyPath, KnownHostsPath: p.KnownHostsPath})
	if err != nil {
		return fmt.Errorf("无法连接目标节点")
	}
	defer c.Close()
	reqIPv6 := snapshot != nil && snapshot.Policy != nil && snapshot.Policy.RequiresIPv6WithOptions(xray.RenderOptions{DefaultProbeURL: p.ProbeURL})
	return c.ApplyXrayConfig(content, p.ConfigPath, false, true, xssh.TransactionOptions{ProbeURL: p.ProbeURL, RequiresIPv6: reqIPv6})
}

func (s *SyncService) DesiredConfigHash(nodeID uint) (string, error) {
	n, err := s.nodeRepo.FindByID(nodeID)
	if err != nil {
		return "", err
	}
	content, _, err := s.buildConfig(n)
	if err != nil {
		return "", err
	}
	return xray.ConfigHash(content), nil
}

func scrubConfigSecretsFromMessage(msg, renderedConfig string) string {
	var obj any
	if json.Unmarshal([]byte(renderedConfig), &obj) != nil {
		return msg
	}
	secrets := make([]string, 0, 8)
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, y := range x {
				if xray.IsSensitiveConfigKey(k) {
					if s, ok := y.(string); ok && s != "" {
						secrets = append(secrets, s)
						// 内核错误可能包含 JSON 转义后的值。
						if encoded, err := json.Marshal(s); err == nil && len(encoded) >= 2 {
							secrets = append(secrets, string(encoded[1:len(encoded)-1]))
						}
					}
				} else {
					walk(y)
				}
			}
		case []any:
			for _, y := range x {
				walk(y)
			}
		}
	}
	walk(obj)
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	replacements := make([]string, 0, len(secrets)*2)
	for _, secret := range secrets {
		replacements = append(replacements, secret, "<masked>")
	}
	return strings.NewReplacer(replacements...).Replace(msg)
}

// Redact recursively, including proxy account passwords, outbound secrets and user UUIDs.
func redactConfig(raw string) string {
	var obj any
	if json.Unmarshal([]byte(raw), &obj) != nil {
		return "<invalid config>"
	}
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, y := range x {
				if xray.IsSensitiveConfigKey(k) {
					x[k] = "<masked>"
				} else {
					walk(y)
				}
			}
		case []any:
			for _, y := range x {
				walk(y)
			}
		}
	}
	walk(obj)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(obj)
	return strings.TrimSuffix(buf.String(), "\n")
}
