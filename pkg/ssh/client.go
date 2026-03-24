package ssh

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Client SSH 客户端，封装连接和常用操作
type Client struct {
	client *ssh.Client
}

// Config SSH 连接配置
type Config struct {
	Host       string
	Port       int
	User       string
	KeyPath    string        // SSH 私钥文件路径
	Timeout    time.Duration // 连接超时，默认 15s
}

// Connect 建立 SSH 连接
func Connect(cfg Config) (*Client, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.Port == 0 {
		cfg.Port = 22
	}

	authMethods, err := buildAuthMethods(cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("加载 SSH 密钥失败 (%s): %w", cfg.KeyPath, err)
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: 生产环境改为 known_hosts 校验
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

// ReadRemoteFile 读取远端文件内容
func (c *Client) ReadRemoteFile(remotePath string) (string, error) {
	out, err := c.Run(fmt.Sprintf("cat %s", remotePath))
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

// TestConnectivity 测试 SSH 连通性，返回往返延迟
func TestConnectivity(cfg Config) (latencyMs int, err error) {
	start := time.Now()
	client, err := Connect(cfg)
	if err != nil {
		return 0, err
	}
	defer client.Close()
	_, err = client.Run("echo ok")
	return int(time.Since(start).Milliseconds()), err
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
