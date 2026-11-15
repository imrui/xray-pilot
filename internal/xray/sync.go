package xray

import (
	"fmt"
	"strings"
	"time"

	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

// RemoteConfigPath 节点上 xray 配置文件路径；GRPCAPIAddress 为节点本地 gRPC API 监听地址
// （API inbound 监听 127.0.0.1:10085，仅本地，通过 SSH 隧道访问）。
const (
	RemoteConfigPath = "/usr/local/etc/xray/config.json"
	GRPCAPIAddress   = "127.0.0.1:10085"
)

const xrayConfigPath = RemoteConfigPath

// SSHParams SSH 连接参数
type SSHParams struct {
	Host            string
	Port            int
	User            string
	KeyPath         string
	KnownHostsPath  string // TOFU known_hosts 文件路径
	BaselineHash    string
	AllowInitial    bool
	BeforeApply     func() error
	ProbeURL        string
	ConfigPath      string
	OnPhase         func(string)
	RollbackContent string
}

// SyncResult 单次同步结果
type SyncResult struct {
	Success       bool
	ElapsedMs     int64
	XrayVersion   string
	Error         string
	ObservedHash  string
	RecoveredHash string
	ProcessID     string
}

// SyncWithFallback 通过 SSH 事务应用完整配置；用户差异由 LiveSync 单独处理。
func SyncWithFallback(params SSHParams, configContent string) *SyncResult {
	start := time.Now()
	configPath := params.ConfigPath
	if configPath == "" {
		configPath = xrayConfigPath
	}

	// SSH 路径：上传完整配置文件并重载 xray
	client, err := xssh.Connect(xssh.Config{
		Host:           params.Host,
		Port:           params.Port,
		User:           params.User,
		KeyPath:        params.KeyPath,
		KnownHostsPath: params.KnownHostsPath,
	})
	if err != nil {
		return &SyncResult{
			Success:   false,
			ElapsedMs: time.Since(start).Milliseconds(),
			Error:     fmt.Sprintf("SSH 连接失败: %v", err),
		}
	}
	defer client.Close()

	if err := client.ApplyXrayConfig(configContent, configPath, true, false, xssh.TransactionOptions{BaselineHash: params.BaselineHash, AllowInitial: params.AllowInitial, BeforeApply: params.BeforeApply, ProbeURL: params.ProbeURL, OnPhase: params.OnPhase, RollbackContent: params.RollbackContent}); err != nil {
		result := &SyncResult{ElapsedMs: time.Since(start).Milliseconds(), Error: fmt.Sprintf("上传配置失败: %v", err)}
		if strings.Contains(err.Error(), "目标文件已存在") {
			if raw, e := client.ReadRemoteFile(configPath); e == nil && ConfigHash(raw) == ConfigHash(configContent) {
				result.ObservedHash = ConfigHash(raw)
			}
		}
		if strings.Contains(err.Error(), "ROLLED_BACK") {
			if raw, e := client.ReadRemoteFile(configPath); e == nil {
				result.RecoveredHash = ConfigHash(raw)
			}
		}
		return result
	}

	// 尝试获取 xray 版本
	ver, _ := client.GetXrayVersion()
	pid, _ := client.RunBounded("systemctl show xray -p MainPID -p ExecMainStartTimestampMonotonic --value", 10*time.Second)

	return &SyncResult{
		Success:     true,
		ElapsedMs:   time.Since(start).Milliseconds(),
		XrayVersion: ver,
		ProcessID:   pid,
	}
}

// CheckNodeHealth 通过 SSH 检测节点健康状态
func CheckNodeHealth(params SSHParams) (latencyMs int, ok bool, err error) {
	return xssh.TestConnectivity(xssh.Config{
		Host:           params.Host,
		Port:           params.Port,
		User:           params.User,
		KeyPath:        params.KeyPath,
		KnownHostsPath: params.KnownHostsPath,
	})
}

// ReadRemoteConfig 读取节点当前 Xray 配置（用于漂移检测）
func ReadRemoteConfig(params SSHParams) (string, error) {
	configPath := params.ConfigPath
	if configPath == "" {
		configPath = xrayConfigPath
	}
	client, err := xssh.Connect(xssh.Config{
		Host:           params.Host,
		Port:           params.Port,
		User:           params.User,
		KeyPath:        params.KeyPath,
		KnownHostsPath: params.KnownHostsPath,
	})
	if err != nil {
		return "", fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	content, err := client.ReadRemoteFile(configPath)
	if err != nil {
		return "", fmt.Errorf("读取远端配置失败: %w", err)
	}
	return content, nil
}
