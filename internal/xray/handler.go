package xray

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/imrui/xray-pilot/internal/xray/handlerpb"
	"github.com/imrui/xray-pilot/internal/xray/handlerpb/common/protocol"
	"github.com/imrui/xray-pilot/internal/xray/handlerpb/common/serial"
	"github.com/imrui/xray-pilot/internal/xray/handlerpb/proxy/trojan"
	"github.com/imrui/xray-pilot/internal/xray/handlerpb/proxy/vless"
	"github.com/imrui/xray-pilot/pkg/types"
)

// HandlerService gRPC 客户端封装：通过 xray proxyman HandlerService.AlterInbound
// 对运行中的 inbound 即时增删用户，无需重启 xray。
//
// 重要边界（见 tmp/v0.4.5-plan.md）：AlterInbound 只改 xray 运行时内存、不写 config.json，
// 重启即丢。调用方必须配合 config 持久化（scp 不重启），本文件只负责 gRPC 即时生效一侧。

// type URL 常量：必须与 xray-core 注册的 message 全名逐字一致，否则 xray 反序列化失败、
// AddUser/RemoveUser 静默无效。取值来源为各 vendored proto 的 package 声明。
const (
	typeURLVlessAccount  = "xray.proxy.vless.Account"
	typeURLTrojanAccount = "xray.proxy.trojan.Account"
	typeURLAddUserOp     = "xray.app.proxyman.command.AddUserOperation"
	typeURLRemoveUserOp  = "xray.app.proxyman.command.RemoveUserOperation"
)

// vlessFlowVision 为 Reality 入站使用的 flow，与 config.go::buildVlessClients 保持一致
const vlessFlowVision = "xtls-rprx-vision"

// toTypedMessage 把 proto message 包成 xray serial.TypedMessage。
// type 为显式传入的全名常量，value 为序列化字节——不依赖 proto 全局注册表。
func toTypedMessage(typeURL string, m proto.Message) (*serial.TypedMessage, error) {
	value, err := proto.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("序列化 %s 失败: %w", typeURL, err)
	}
	return &serial.TypedMessage{Type: typeURL, Value: value}, nil
}

// BuildVlessAccount 构造 VLESS 账号 TypedMessage。flow 为空表示 WS+TLS，xtls-rprx-vision 表示 Reality。
func BuildVlessAccount(uuid, flow string) (*serial.TypedMessage, error) {
	return toTypedMessage(typeURLVlessAccount, &vless.Account{
		Id:         uuid,
		Flow:       flow,
		Encryption: "none",
	})
}

// BuildTrojanAccount 构造 Trojan 账号 TypedMessage（密码即用户 UUID，与 config.go 一致）。
func BuildTrojanAccount(password string) (*serial.TypedMessage, error) {
	return toTypedMessage(typeURLTrojanAccount, &trojan.Account{Password: password})
}

// BuildAccountForProtocol 按协议类型构造账号 TypedMessage，映射与 config.go 的 inbound 生成对齐：
//   - vless-reality → vless.Account{flow: xtls-rprx-vision}
//   - vless-ws-tls  → vless.Account{flow: ""}
//   - trojan        → trojan.Account{password}
//
// Hysteria2 不在 xray-core，返回错误（调用方应跳过）。
func BuildAccountForProtocol(protocolType, uuid string) (*serial.TypedMessage, error) {
	switch protocolType {
	case types.ProtocolVlessReality:
		return BuildVlessAccount(uuid, vlessFlowVision)
	case types.ProtocolVlessWSTLS:
		return BuildVlessAccount(uuid, "")
	case types.ProtocolTrojan:
		return BuildTrojanAccount(uuid)
	default:
		return nil, fmt.Errorf("协议 %s 不支持 gRPC 用户增删", protocolType)
	}
}

// AddUser 对指定 inbound（按 tag 定位）即时新增一个用户。account 由 BuildAccountForProtocol 构造，
// email 用作 xray 侧用户标识（= username，与 stats 计数 email 一致）。
func AddUser(ctx context.Context, conn *grpc.ClientConn, inboundTag, email string, account *serial.TypedMessage) error {
	op := &handlerpb.AddUserOperation{
		User: &protocol.User{
			Email:   email,
			Account: account,
		},
	}
	return alterInbound(ctx, conn, inboundTag, typeURLAddUserOp, op,
		fmt.Sprintf("AddUser(tag=%s, email=%s)", inboundTag, email))
}

// RemoveUser 对指定 inbound 即时移除一个用户（按 email 定位）。
func RemoveUser(ctx context.Context, conn *grpc.ClientConn, inboundTag, email string) error {
	op := &handlerpb.RemoveUserOperation{Email: email}
	return alterInbound(ctx, conn, inboundTag, typeURLRemoveUserOp, op,
		fmt.Sprintf("RemoveUser(tag=%s, email=%s)", inboundTag, email))
}

// alterInbound 把 operation 包成 TypedMessage 后调用 HandlerService.AlterInbound。
func alterInbound(ctx context.Context, conn *grpc.ClientConn, inboundTag, opTypeURL string, op proto.Message, label string) error {
	opMsg, err := toTypedMessage(opTypeURL, op)
	if err != nil {
		return err
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cli := handlerpb.NewHandlerServiceClient(conn)
	if _, err := cli.AlterInbound(callCtx, &handlerpb.AlterInboundRequest{
		Tag:       inboundTag,
		Operation: opMsg,
	}); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}
