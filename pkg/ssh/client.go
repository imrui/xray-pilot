package ssh

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// knownHostsMu 保护 known_hosts 文件的并发写入
var knownHostsMu sync.Mutex

// Client SSH 客户端，封装连接和常用操作
type Client struct {
	client *ssh.Client
}

// Config SSH 连接配置
type Config struct {
	Host           string
	Port           int
	User           string
	KeyPath        string        // SSH 私钥文件路径
	KnownHostsPath string        // known_hosts 文件路径（TOFU），留空则使用 data/known_hosts
	Timeout        time.Duration // 连接超时，默认 15s
}

// Connect 建立 SSH 连接（TOFU known_hosts 校验：首次自动信任并记录，后续严格比对）
func Connect(cfg Config) (*Client, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if cfg.KnownHostsPath == "" {
		cfg.KnownHostsPath = "data/known_hosts"
	}

	authMethods, err := buildAuthMethods(cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("加载 SSH 密钥失败 (%s): %w", cfg.KeyPath, err)
	}

	hostKeyCallback, err := buildTOFUCallback(cfg.KnownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("初始化 known_hosts 失败: %w", err)
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         cfg.Timeout,
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败 (%s): %w", addr, err)
	}
	return &Client{client: client}, nil
}

// Close 关闭连接
func (c *Client) Close() error {
	return c.client.Close()
}

// Run 执行远端命令，返回合并输出和错误
func (c *Client) Run(cmd string) (string, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer sess.Close()

	var buf bytes.Buffer
	sess.Stdout = &buf
	sess.Stderr = &buf

	if err := sess.Run(cmd); err != nil {
		return buf.String(), fmt.Errorf("命令执行失败 [%s]: %w\noutput: %s", cmd, err, buf.String())
	}
	return buf.String(), nil
}

// UploadContent 将字符串内容上传到远端路径（通过 stdin pipe 写入）
func (c *Client) UploadContent(content, remotePath string) error {
	// 先写入临时文件，再原子 mv，避免 xray 读取到写一半的文件
	tmpPath := remotePath + ".tmp"

	sess, err := c.client.NewSession()
	if err != nil {
		return fmt.Errorf("创建 SSH 会话失败: %w", err)
	}
	defer sess.Close()

	var stderr bytes.Buffer
	sess.Stderr = &stderr

	// 确保目标目录存在
	mkdirSess, _ := c.client.NewSession()
	if mkdirSess != nil {
		dir := remotePath[:strings.LastIndex(remotePath, "/")]
		_ = mkdirSess.Run(fmt.Sprintf("mkdir -p %s", dir))
		mkdirSess.Close()
	}

	// 通过 stdin pipe 写入临时文件
	stdin, err := sess.StdinPipe()
	if err != nil {
		return fmt.Errorf("获取 stdin pipe 失败: %w", err)
	}

	if err := sess.Start(fmt.Sprintf("cat > %s", tmpPath)); err != nil {
		return fmt.Errorf("启动写入命令失败: %w", err)
	}

	if _, err := io.WriteString(stdin, content); err != nil {
		return fmt.Errorf("写入内容失败: %w", err)
	}
	stdin.Close()

	if err := sess.Wait(); err != nil {
		return fmt.Errorf("写入远端文件失败: %w\nstderr: %s", err, stderr.String())
	}

	// 原子替换
	if _, err := c.Run(fmt.Sprintf("mv %s %s", tmpPath, remotePath)); err != nil {
		return fmt.Errorf("移动临时文件失败: %w", err)
	}
	return nil
}

// ReadRemoteFile 读取远端文件内容（stderr 重定向至 /dev/null，防止错误信息污染文件内容 hash）
func (c *Client) ReadRemoteFile(remotePath string) (string, error) {
	out, err := c.Run(fmt.Sprintf("cat %s 2>/dev/null", remotePath))
	if err != nil {
		return "", fmt.Errorf("读取远端文件失败 (%s): %w", remotePath, err)
	}
	return out, nil
}

// ReloadXray 重载 Xray 服务（systemctl 优先，回退 service）
func (c *Client) ReloadXray() error {
	// 尝试 systemctl restart
	if _, err := c.Run("systemctl restart xray"); err == nil {
		return nil
	}
	// 回退到 service
	if _, err := c.Run("service xray restart"); err == nil {
		return nil
	}
	// 回退到直接 kill -HUP
	_, err := c.Run("kill -HUP $(pidof xray) 2>/dev/null || true")
	return err
}

// TestConnectivity 测试 SSH 连通性，返回往返延迟、是否成功和错误
func TestConnectivity(cfg Config) (latencyMs int, ok bool, err error) {
	start := time.Now()
	client, connErr := Connect(cfg)
	if connErr != nil {
		return 0, false, connErr
	}
	defer client.Close()
	if _, runErr := client.Run("echo ok"); runErr != nil {
		return int(time.Since(start).Milliseconds()), false, runErr
	}
	return int(time.Since(start).Milliseconds()), true, nil
}

// GetXrayVersion 读取远端节点 Xray 版本号
func (c *Client) GetXrayVersion() (string, error) {
	out, err := c.Run("xray version 2>/dev/null | head -1 | awk '{print $2}'")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// buildAuthMethods 从密钥文件路径构建认证方式
func buildAuthMethods(keyPath string) ([]ssh.AuthMethod, error) {
	if keyPath == "" {
		// 回退到默认 ~/.ssh/id_rsa 和 ~/.ssh/id_ed25519
		for _, defaultKey := range []string{
			os.ExpandEnv("$HOME/.ssh/id_ed25519"),
			os.ExpandEnv("$HOME/.ssh/id_rsa"),
		} {
			if method, err := loadKeyFile(defaultKey); err == nil {
				return []ssh.AuthMethod{method}, nil
			}
		}
		return nil, fmt.Errorf("未指定 SSH 密钥且默认密钥不存在")
	}
	method, err := loadKeyFile(keyPath)
	if err != nil {
		return nil, err
	}
	return []ssh.AuthMethod{method}, nil
}

func loadKeyFile(path string) (ssh.AuthMethod, error) {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取密钥文件失败 (%s): %w", path, err)
	}
	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("解析密钥文件失败 (%s): %w", path, err)
	}
	return ssh.PublicKeys(signer), nil
}

// buildTOFUCallback 构建 TOFU（Trust On First Use）主机密钥校验回调：
// 首次连接未知主机时自动信任并追加至 known_hosts；后续连接严格比对，密钥变更则拒绝。
func buildTOFUCallback(path string) (ssh.HostKeyCallback, error) {
	// 确保 known_hosts 文件及目录存在
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("创建 known_hosts 目录失败: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("打开 known_hosts 文件失败 (%s): %w", path, err)
	}
	f.Close()

	// 加载已知主机列表
	cb, err := knownhosts.New(path)
	if err != nil {
		return nil, fmt.Errorf("加载 known_hosts 失败: %w", err)
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := cb(hostname, remote, key)
		if err == nil {
			return nil // 已知且匹配，直接通过
		}
		// 密钥已变更（Want 非空 = 已知主机但密钥不符，可能 MITM），拒绝连接
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) && len(keyErr.Want) > 0 {
			return fmt.Errorf("主机密钥已变更，拒绝连接（可能存在中间人攻击）[%s]: %w", hostname, err)
		}
		// 未知主机：TOFU — 追加并接受
		knownHostsMu.Lock()
		defer knownHostsMu.Unlock()
		wf, werr := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if werr != nil {
			return fmt.Errorf("写入 known_hosts 失败: %w", werr)
		}
		defer wf.Close()
		line := knownhosts.Line([]string{hostname}, key)
		if _, werr = fmt.Fprintln(wf, line); werr != nil {
			return fmt.Errorf("写入主机密钥失败: %w", werr)
		}
		return nil
	}, nil
}
