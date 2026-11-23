package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func TestConnectContextCancelsStalledHandshake(t *testing.T) {
	dir := t.TempDir()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := gossh.MarshalPrivateKey(key, "test")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "key")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		c, err := ConnectContext(ctx, Config{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, User: "test", KeyPath: keyPath, KnownHostsPath: filepath.Join(dir, "hosts")})
		if c != nil {
			_ = c.Close()
		}
		result <- err
	}()
	_ = listener.SetDeadline(time.Now().Add(5 * time.Second))
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// 对端已接受 TCP，但一直没有返回 SSH 握手数据。
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SSH handshake did not stop on cancellation")
	}
}
