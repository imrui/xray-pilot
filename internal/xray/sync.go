package xray

import (
	"fmt"
	"time"

	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

const xrayConfigPath = "/usr/local/etc/xray/config.json"

// SSHParams SSH 连接参数
type SSHParams struct {
	Host           string
	Port           int
	User           string
	KeyPath        string
	KnownHostsPath string // TOFU known_hosts 文件路径
}

// SyncResult 单次同步结果
type SyncResult struct {
	Success     bool
	ElapsedMs   int64
	XrayVersion string
	Error       string
}

// SyncWithFallback 同步 Xray 配置：优先 gRPC，失败则 SSH 推送配置文件
// 当前版本仅实现 SSH 路径（gRPC 路径为 TODO）
func SyncWithFallback(params SSHParams, configContent string) *SyncResult {
	start := time.Now()

	// TODO: 尝试 gRPC 路径（通过 SSH 端口转发 10085，调用 HandlerService 动态更新用户）
	// if ver, err := syncViaGRPC(params); err == nil {
	//     return &SyncResult{Success: true, ElapsedMs: ..., XrayVersion: ver}
	// }

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

	if err := client.UploadContent(configContent, xrayConfigPath); err != nil {
		return &SyncResult{
			Success:   false,
			ElapsedMs: time.Since(start).Milliseconds(),
			Error:     fmt.Sprintf("上传配置失败: %v", err),
		}
	}

	if err := client.ReloadXray(); err != nil {
		return &SyncResult{
			Success:   false,
			ElapsedMs: time.Since(start).Milliseconds(),
			Error:     fmt.Sprintf("重载 xray 失败: %v", err),
		}
	}

	// 尝试获取 xray 版本
	ver, _ := client.GetXrayVersion()

	return &SyncResult{
		Success:     true,
		ElapsedMs:   time.Since(start).Milliseconds(),
		XrayVersion: ver,
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

	content, err := client.ReadRemoteFile(xrayConfigPath)
	if err != nil {
		return "", fmt.Errorf("读取远端配置失败: %w", err)
	}
	return content, nil
}
