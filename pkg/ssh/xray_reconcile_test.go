package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
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
		name                              string
		verified, manual, initial         bool
		phase                             string
		recovery, badHash, changedProcess bool
		recheck, live, badPath            bool
	}{
		{name: "verified", verified: true}, {name: "unknown"}, {name: "manual", manual: true}, {name: "manual-with-initial-permit", manual: true, initial: true},
		{name: "restored", phase: "restored", recovery: true, verified: true},
		{name: "restored-file-changed", phase: "restored", recovery: true, badHash: true},
		{name: "restored-process-changed", phase: "restored", recovery: true, changedProcess: true},
		{name: "restored-is-not-healthy", phase: "restored"},
		{name: "restored-rechecked", phase: "restored", recheck: true, verified: true},
		{name: "live-rechecked", live: true, recheck: true, verified: true},
		{name: "live-process-changed", live: true, changedProcess: true},
		{name: "healthy-is-not-restored", phase: "verified", recovery: true},
		{name: "matching-file-wrong-path", phase: "verified", badPath: true},
	} {
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
			phase := tc.phase
			if phase == "" {
				phase = "verified"
			}
			receiptHash := hex.EncodeToString(sum[:])
			if tc.badHash {
				receiptHash = strings.Repeat("0", 64)
			}
			receipt, _ := json.Marshal(map[string]string{"phase": phase, "hash": receiptHash, "process": "17\n123"})
			if tc.live || !verified && tc.phase == "" {
				receipt = []byte("persisted")
			}
			var mutations atomic.Int32
			var probes atomic.Int32
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
							case strings.HasPrefix(cmd.Command, "test -f "):
								data := []byte(raw)
								if strings.Contains(cmd.Command, ".pilot-config-state") {
									data = receipt
								}
								response = string(data)
							case strings.HasPrefix(cmd.Command, "systemctl is-active --quiet xray && systemctl show"):
								response = "17\n123"
								if tc.changedProcess {
									response = "18\n456"
								}
							case strings.Contains(cmd.Command, "PILOT:VERIFY_ONLY"):
								probes.Add(1)
								if strings.Contains(cmd.Command, "systemctl restart") || strings.Contains(cmd.Command, "mv -f") || strings.Contains(cmd.Command, "cp -p") {
									mutations.Add(1)
								}
								io.Copy(io.Discard, ch)
								status = 1
								if tc.recheck && !tc.changedProcess && !strings.Contains(cmd.Command, "123\n") {
									response, status = "VERIFIED", 0
								}
							case strings.Contains(cmd.Command, "PILOT:CHECK_PATH"):
								if tc.badPath {
									status = 1
								}
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
			if tc.recovery {
				_, err = client.VerifyRecoveryReceipt(candidate, "/etc/xray/config.json")
			} else {
				opt := TransactionOptions{AllowInitial: tc.initial}
				if tc.live {
					opt.ConfirmedProcess = "17\n123\n"
				}
				err = client.ApplyXrayConfig(candidate, "/etc/xray/config.json", true, false, opt)
			}
			client.Close()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("SSH server stuck")
			}
			if mutations.Load() != 0 {
				t.Fatal("matching target replayed a write or restart")
			}
			if tc.recheck && probes.Load() != 1 {
				t.Fatal("restored/live receipt bypassed health recheck")
			}
			if tc.badPath && (err == nil || !strings.Contains(err.Error(), "PATH_MISMATCH")) {
				t.Fatalf("matching bytes bypassed service path validation: %v", err)
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
			if !verified && err == nil {
				t.Fatalf("unverified target reported success: %v", err)
			}
		})
	}
}
