package ssh

import (
	"context"
	"fmt"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// DialGRPC 通过 SSH 隧道连接远端节点上的 gRPC 服务（h2c 明文）
//
// 典型场景：远端 xray-core gRPC API 监听 127.0.0.1:10085（仅本地），通过 SSH 通道转发后
// 在本地拨号即可访问，无需把 gRPC 端口对外开放。
//
// 返回的 *grpc.ClientConn 与底层 SSH conn 生命周期解耦，但底层 ssh.Client 关闭后 gRPC 立即失效。
// 调用方应在不再使用时显式调用 cc.Close()
//
// 关键点：
//   - 使用 passthrough:/// resolver 避免 grpc-go 把 target 丢给 DNS 解析
//   - WithContextDialer 把所有拨号转给 ssh.Client.Dial，由 SSH 通道完成 TCP 建立
//   - 使用 keepalive 防止 SSH 服务端空闲超时断开
func (c *Client) DialGRPC(ctx context.Context, remoteHostPort string) (*grpc.ClientConn, error) {
	dialer := func(_ context.Context, addr string) (net.Conn, error) {
		return c.client.Dial("tcp", addr)
	}

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cc, err := grpc.DialContext(dialCtx, "passthrough:///"+remoteHostPort,
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("dial grpc via ssh tunnel (%s): %w", remoteHostPort, err)
	}
	return cc, nil
}
