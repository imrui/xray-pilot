package service

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// defaultOutboundIPProbeURL 自动探测面板出网 IP 的外部服务。
// 选 ipify 是因为响应体就是纯 IP 文本、稳定、无配额限制。
// 可通过环境变量 XRAY_PILOT_OUTBOUND_PROBE_URL 覆盖，便于私有部署接入内部探针。
const defaultOutboundIPProbeURL = "https://api.ipify.org"

// PanelService 面板自身相关的运维信息
//
// 当前仅承载「面板出网 IP」自动探测能力，用于一键接入流程提示用户在
// 节点防火墙放行哪个 IP。后续若有其他面板自检能力（版本、链路自检）可叠加。
type PanelService struct {
	settingSvc *SettingService
	httpClient *http.Client
}

func NewPanelService() *PanelService {
	return &PanelService{
		settingSvc: NewSettingService(),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

// DetectOutboundIP 通过外部探针拿到 panel 的出网 IP，写入 setting。
//
// 失败时不覆盖既有 auto 值（保留上一次成功结果），避免短暂网络抖动让 UI 突然空白。
// 返回值是本次拿到的 IP；失败时返回 ""。
func (s *PanelService) DetectOutboundIP(ctx context.Context) (string, error) {
	url := strings.TrimSpace(os.Getenv("XRAY_PILOT_OUTBOUND_PROBE_URL"))
	if url == "" {
		url = defaultOutboundIPProbeURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", errors.New("探针返回非 2xx")
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))
	if ip == "" || net.ParseIP(ip) == nil {
		return "", errors.New("探针返回内容不是合法 IP")
	}
	if err := s.settingSvc.BatchUpdate(map[string]string{
		KeyPanelOutboundIPAuto: ip,
	}); err != nil {
		return ip, err
	}
	return ip, nil
}

// EffectiveOutboundIP 返回当前生效的面板出网 IP。
// 优先级：手动覆盖 > 自动探测。两者都空时返回 ""（前端文案降级为通用提示）。
func (s *PanelService) EffectiveOutboundIP() string {
	if manual := strings.TrimSpace(s.settingSvc.Get(KeyPanelOutboundIPManual)); manual != "" {
		return manual
	}
	return strings.TrimSpace(s.settingSvc.Get(KeyPanelOutboundIPAuto))
}
