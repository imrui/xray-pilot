package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

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
	p := s.sshParams(node)
	c, err := xssh.Connect(xssh.Config{Host: p.Host, Port: p.Port, User: p.User, KeyPath: p.KeyPath, KnownHostsPath: p.KnownHostsPath})
	if err != nil {
		return fmt.Errorf("无法连接目标节点")
	}
	defer c.Close()
	return c.ApplyXrayConfig(content, p.ConfigPath, false, true, xssh.TransactionOptions{ProbeURL: p.ProbeURL})
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

// Redact recursively, including proxy account passwords and user UUIDs.
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
				switch k {
				case "privateKey", "password", "id", "email", "pass", "user", "secretKey", "preSharedKey":
					x[k] = "<masked>"
				default:
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
