package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"strings"
	"time"
)

// VerifyConfigReceipt confirms the disk hash and the exact process instance that
// passed the protocol probe. A matching file alone is deliberately insufficient.
func (c *Client) VerifyConfigReceipt(content, configPath string) (string, error) {
	raw, err := c.ReadRemoteFile(path.Join(path.Dir(configPath), ".pilot-config-state"))
	if err != nil {
		return "", fmt.Errorf("缺少完整应用回执")
	}
	var receipt struct{ Phase, Hash, Process string }
	if json.Unmarshal([]byte(raw), &receipt) != nil || receipt.Phase != "verified" {
		return "", fmt.Errorf("远端尚未确认健康")
	}
	sum := sha256.Sum256([]byte(content))
	if receipt.Hash != hex.EncodeToString(sum[:]) {
		return "", fmt.Errorf("文件与应用回执不一致")
	}
	process, err := c.RunBounded("systemctl is-active --quiet xray && systemctl show xray -p MainPID -p ExecMainStartTimestampMonotonic --value", 10*time.Second)
	if err != nil || strings.TrimSpace(process) != receipt.Process {
		return "", fmt.Errorf("运行进程已变化，需重新验证")
	}
	return receipt.Process, nil
}

func (c *Client) RunBounded(command string, duration time.Duration) (string, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	var output limitedOutput
	sess.Stdout = &output
	sess.Stderr = &output
	timer := time.AfterFunc(duration, func() { _ = sess.Close() })
	defer timer.Stop()
	if err := sess.Run(command); err != nil {
		return "", fmt.Errorf("远端检查未完成")
	}
	return output.text, nil
}
