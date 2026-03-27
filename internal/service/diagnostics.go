package service

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/imrui/xray-pilot/config"
)

type DiagnosticStatus string

const (
	DiagnosticOK      DiagnosticStatus = "ok"
	DiagnosticWarning DiagnosticStatus = "warning"
	DiagnosticError   DiagnosticStatus = "error"
)

type DiagnosticItem struct {
	Key        string           `json:"key"`
	Label      string           `json:"label"`
	Status     DiagnosticStatus `json:"status"`
	Detail     string           `json:"detail"`
	Suggestion string           `json:"suggestion,omitempty"`
	Value      string           `json:"value,omitempty"`
}

type DiagnosticsResult struct {
	Summary struct {
		OK      int `json:"ok"`
		Warning int `json:"warning"`
		Error   int `json:"error"`
	} `json:"summary"`
	Items []DiagnosticItem `json:"items"`
}

type DiagnosticsService struct {
	settings *SettingService
}

func NewDiagnosticsService() *DiagnosticsService {
	return &DiagnosticsService{settings: NewSettingService()}
}

func (s *DiagnosticsService) Run() DiagnosticsResult {
	items := []DiagnosticItem{
		s.checkDatabase(),
		s.checkSubscriptionBaseURL(),
		s.checkSSHDefaultKeyPath(),
		s.checkKnownHostsPath(),
		s.checkWorkingConfig(),
	}

	var result DiagnosticsResult
	result.Items = items
	for _, item := range items {
		switch item.Status {
		case DiagnosticOK:
			result.Summary.OK++
		case DiagnosticWarning:
			result.Summary.Warning++
		case DiagnosticError:
			result.Summary.Error++
		}
	}
	return result
}

func (s *DiagnosticsService) checkDatabase() DiagnosticItem {
	cfg := config.Global.Database
	item := DiagnosticItem{
		Key:    "database",
		Label:  "数据库路径",
		Value:  cfg.DSN,
		Status: DiagnosticOK,
	}

	if cfg.Driver != "sqlite" {
		item.Detail = "当前使用外部数据库驱动，未执行本地文件写入检查。"
		return item
	}

	targetDir := filepath.Dir(cfg.DSN)
	if targetDir == "." || targetDir == "" {
		targetDir = "."
	}
	if err := ensureDirWritable(targetDir, ".xray-pilot-db-check"); err != nil {
		item.Status = DiagnosticError
		item.Detail = "SQLite 数据目录当前不可写。"
		item.Suggestion = err.Error()
		return item
	}

	item.Detail = "SQLite 数据目录可写。"
	return item
}

func (s *DiagnosticsService) checkSubscriptionBaseURL() DiagnosticItem {
	baseURL := strings.TrimSpace(s.settings.Get(KeySubscriptionBaseURL))
	item := DiagnosticItem{
		Key:    "subscription_base_url",
		Label:  "订阅基址",
		Value:  baseURL,
		Status: DiagnosticOK,
	}

	if baseURL == "" {
		item.Status = DiagnosticWarning
		item.Detail = "当前使用请求头自动推断订阅地址。"
		item.Suggestion = "如果服务部署在 Nginx / CDN / HTTPS 反向代理后，建议显式设置 subscription.base_url。"
		return item
	}

	if strings.HasPrefix(baseURL, "https://") {
		item.Detail = "订阅基址已显式配置为 HTTPS。"
		return item
	}

	if strings.HasPrefix(baseURL, "http://") {
		item.Status = DiagnosticWarning
		item.Detail = "订阅基址已配置，但当前不是 HTTPS。"
		item.Suggestion = "如果用户通过 HTTPS 访问后台，建议把 subscription.base_url 改成 https:// 域名。"
		return item
	}

	item.Status = DiagnosticWarning
	item.Detail = "订阅基址已配置，但格式看起来不是标准 URL。"
	item.Suggestion = "示例：https://example.com"
	return item
}

func (s *DiagnosticsService) checkSSHDefaultKeyPath() DiagnosticItem {
	keyPath := strings.TrimSpace(s.settings.Get(KeySSHDefaultKeyPath))
	item := DiagnosticItem{
		Key:    "ssh_default_key_path",
		Label:  "默认 SSH 私钥",
		Value:  keyPath,
		Status: DiagnosticOK,
	}

	if keyPath == "" {
		item.Status = DiagnosticWarning
		item.Detail = "当前未配置默认 SSH 私钥路径。"
		item.Suggestion = "建议在 systemd 部署中使用 /etc/xray-pilot/ssh/id_ed25519，并在系统设置中保存。"
		return item
	}

	info, err := os.Stat(keyPath)
	if err != nil {
		item.Status = DiagnosticError
		item.Detail = "默认 SSH 私钥路径不存在或无法访问。"
		item.Suggestion = "确认文件存在，并保证运行用户能读取该路径。"
		return item
	}
	if info.IsDir() {
		item.Status = DiagnosticError
		item.Detail = "默认 SSH 私钥路径指向了目录，不是文件。"
		item.Suggestion = "请填写实际私钥文件路径，例如 /etc/xray-pilot/ssh/id_ed25519。"
		return item
	}
	f, err := os.Open(keyPath)
	if err != nil {
		item.Status = DiagnosticError
		item.Detail = "默认 SSH 私钥文件存在，但当前进程无法读取。"
		item.Suggestion = "请检查文件 owner/group 和权限，确保服务用户可以读取。"
		return item
	}
	_ = f.Close()

	item.Detail = "默认 SSH 私钥文件存在，当前进程可读取。"
	return item
}

func (s *DiagnosticsService) checkKnownHostsPath() DiagnosticItem {
	path := strings.TrimSpace(s.settings.Get(KeySSHKnownHostsPath))
	item := DiagnosticItem{
		Key:    "ssh_known_hosts_path",
		Label:  "known_hosts 路径",
		Value:  path,
		Status: DiagnosticOK,
	}

	if path == "" {
		item.Status = DiagnosticWarning
		item.Detail = "当前未配置 known_hosts 路径，将依赖程序默认值。"
		item.Suggestion = "建议显式配置为 /var/lib/xray-pilot/known_hosts。"
		return item
	}

	dir := filepath.Dir(path)
	if err := ensureDirWritable(dir, ".xray-pilot-known-hosts-check"); err != nil {
		item.Status = DiagnosticError
		item.Detail = "known_hosts 目录不可写。"
		item.Suggestion = err.Error()
		return item
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		item.Status = DiagnosticError
		item.Detail = "known_hosts 文件不可读写。"
		item.Suggestion = "请检查文件和上级目录权限，确保服务用户可以创建和写入。"
		return item
	}
	_ = f.Close()

	item.Detail = "known_hosts 文件可创建且可读写。"
	return item
}

func (s *DiagnosticsService) checkWorkingConfig() DiagnosticItem {
	item := DiagnosticItem{
		Key:    "config_file",
		Label:  "当前配置文件",
		Value:  "config.yaml",
		Status: DiagnosticOK,
	}

	f, err := os.Open("config.yaml")
	if err != nil {
		item.Status = DiagnosticError
		item.Detail = "当前工作目录下的 config.yaml 不可读取。"
		item.Suggestion = "请确认 WorkingDirectory 正确，或检查 /etc/xray-pilot/config.yaml 权限。"
		return item
	}
	_ = f.Close()

	item.Detail = "当前进程可以读取 config.yaml。"
	return item
}

func ensureDirWritable(dir, probeName string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	probePath := filepath.Join(dir, probeName)
	f, err := os.OpenFile(probePath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	_ = f.Close()
	_ = os.Remove(probePath)
	return nil
}
