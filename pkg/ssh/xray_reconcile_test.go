package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	gossh "golang.org/x/crypto/ssh"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// 使用真实本地 SSH 会话验证响应丢失后的路径，不允许发出任何修改命令。
func TestMatchingTargetOnlyReconciles(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		verified, manual, initial bool
	}{{name: "verified", verified: true}, {name: "unknown"}, {name: "manual", manual: true}, {name: "manual-with-initial-permit", manual: true, initial: true}} {
		t.Run(tc.name, func(t *testing.T) {
			verified := tc.verified
			_, key, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			signer, err := gossh.NewSignerFromKey(key)
			if err != nil {
				t.Fatal(err)
			}
			serverConfig := &gossh.ServerConfig{NoClientAuth: true}
			serverConfig.AddHostKey(signer)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			raw := `{"inbounds":[],"outbounds":[{"tag":"direct","protocol":"freedom"}]}`
			sum := sha256.Sum256([]byte(raw))
			receipt, _ := json.Marshal(map[string]string{"phase": "verified", "hash": hex.EncodeToString(sum[:]), "process": "17\n123"})
			if !verified {
				receipt = []byte("persisted")
			}
			var mutations atomic.Int32
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, e := listener.Accept()
				if e != nil {
					return
				}
				defer conn.Close()
				server, channels, requests, e := gossh.NewServerConn(conn, serverConfig)
				if e != nil {
					return
				}
				defer server.Close()
				go gossh.DiscardRequests(requests)
				for incoming := range channels {
					ch, reqs, e := incoming.Accept()
					if e != nil {
						return
					}
					go func() {
						defer ch.Close()
						for req := range reqs {
							if req.Type != "exec" {
								req.Reply(false, nil)
								continue
							}
							var cmd struct{ Command string }
							gossh.Unmarshal(req.Payload, &cmd)
							req.Reply(true, nil)
							response := ""
							status := uint32(0)
							switch {
							case strings.HasPrefix(cmd.Command, "test -s "):
								data := []byte(raw)
								if strings.Contains(cmd.Command, ".pilot-config-state") {
									data = receipt
								}
								response = base64.StdEncoding.EncodeToString(data)
							case strings.HasPrefix(cmd.Command, "systemctl is-active --quiet xray && systemctl show"):
								response = "17\n123"
							default:
								mutations.Add(1)
								status = 1
							}
							io.WriteString(ch, response)
							ch.SendRequest("exit-status", false, gossh.Marshal(struct{ Status uint32 }{status}))
							return
						}
					}()
				}
			}()
			connection, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{User: "test", HostKeyCallback: gossh.FixedHostKey(signer.PublicKey()), Timeout: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			client := &Client{client: connection}
			candidate := raw
			if tc.manual {
				candidate = strings.Replace(raw, "direct", "new-direct", 1)
			}
			err = client.ApplyXrayConfig(candidate, "/etc/xray/config.json", true, false, TransactionOptions{AllowInitial: tc.initial})
			client.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("SSH server stuck")
			}
			if mutations.Load() != 0 {
				t.Fatal("matching target replayed a write or restart")
			}
			if tc.manual {
				if err == nil || !strings.Contains(err.Error(), "DRIFT_CONFLICT") {
					t.Fatalf("manual config overwritten: %v", err)
				}
				return
			}
			if verified && err != nil {
				t.Fatal(err)
			}
			if !verified && (err == nil || !strings.Contains(err.Error(), "未完成")) {
				t.Fatalf("unverified target reported success: %v", err)
			}
		})
	}
}
