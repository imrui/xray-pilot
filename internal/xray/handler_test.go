package xray

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/imrui/xray-pilot/internal/xray/handlerpb"
	"github.com/imrui/xray-pilot/internal/xray/handlerpb/common/protocol"
	"github.com/imrui/xray-pilot/internal/xray/handlerpb/proxy/trojan"
	"github.com/imrui/xray-pilot/internal/xray/handlerpb/proxy/vless"
	"github.com/imrui/xray-pilot/pkg/types"
)

// TestTypeURLConstants 锁死 type URL 字面值——写错一个字符 xray 就会静默丢弃 AddUser/RemoveUser。
// 这些值来源于 vendored proto 的 package 声明，必须与 xray-core 逐字一致。
func TestTypeURLConstants(t *testing.T) {
	cases := map[string]string{
		"vless account":  typeURLVlessAccount,
		"trojan account": typeURLTrojanAccount,
		"add user op":    typeURLAddUserOp,
		"remove user op": typeURLRemoveUserOp,
	}
	want := map[string]string{
		"vless account":  "xray.proxy.vless.Account",
		"trojan account": "xray.proxy.trojan.Account",
		"add user op":    "xray.app.proxyman.command.AddUserOperation",
		"remove user op": "xray.app.proxyman.command.RemoveUserOperation",
	}
	for k, got := range cases {
		if got != want[k] {
			t.Errorf("%s type URL = %q, want %q", k, got, want[k])
		}
	}
}

func TestBuildVlessAccount(t *testing.T) {
	tm, err := BuildVlessAccount("uuid-123", vlessFlowVision)
	if err != nil {
		t.Fatalf("BuildVlessAccount: %v", err)
	}
	if tm.Type != typeURLVlessAccount {
		t.Errorf("Type = %q, want %q", tm.Type, typeURLVlessAccount)
	}
	// 反序列化 value 校验字段编码正确
	var acc vless.Account
	if err := proto.Unmarshal(tm.Value, &acc); err != nil {
		t.Fatalf("unmarshal vless account: %v", err)
	}
	if acc.Id != "uuid-123" || acc.Flow != vlessFlowVision || acc.Encryption != "none" {
		t.Errorf("account = %+v, want id=uuid-123 flow=%s encryption=none", &acc, vlessFlowVision)
	}
}

func TestBuildTrojanAccount(t *testing.T) {
	tm, err := BuildTrojanAccount("pass-456")
	if err != nil {
		t.Fatalf("BuildTrojanAccount: %v", err)
	}
	if tm.Type != typeURLTrojanAccount {
		t.Errorf("Type = %q, want %q", tm.Type, typeURLTrojanAccount)
	}
	var acc trojan.Account
	if err := proto.Unmarshal(tm.Value, &acc); err != nil {
		t.Fatalf("unmarshal trojan account: %v", err)
	}
	if acc.Password != "pass-456" {
		t.Errorf("password = %q, want pass-456", acc.Password)
	}
}

func TestBuildAccountForProtocol(t *testing.T) {
	cases := []struct {
		protocol string
		wantType string
		wantFlow string // 仅 vless 校验
	}{
		{types.ProtocolVlessReality, typeURLVlessAccount, vlessFlowVision},
		{types.ProtocolVlessWSTLS, typeURLVlessAccount, ""},
		{types.ProtocolTrojan, typeURLTrojanAccount, ""},
	}
	for _, c := range cases {
		t.Run(c.protocol, func(t *testing.T) {
			tm, err := BuildAccountForProtocol(c.protocol, "uuid-x")
			if err != nil {
				t.Fatalf("BuildAccountForProtocol(%s): %v", c.protocol, err)
			}
			if tm.Type != c.wantType {
				t.Fatalf("Type = %q, want %q", tm.Type, c.wantType)
			}
			if c.wantType == typeURLVlessAccount {
				var acc vless.Account
				if err := proto.Unmarshal(tm.Value, &acc); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if acc.Flow != c.wantFlow {
					t.Errorf("flow = %q, want %q", acc.Flow, c.wantFlow)
				}
			}
		})
	}
}

func TestBuildAccountForProtocol_Hysteria2Rejected(t *testing.T) {
	if _, err := BuildAccountForProtocol(types.ProtocolHysteria2, "uuid-x"); err == nil {
		t.Fatal("Hysteria2 应返回错误（不在 xray-core），但未报错")
	}
}

// TestAddUserOperationRoundTrip 验证 AddUserOperation → TypedMessage → 还原后 user/account 完整。
// 模拟 xray 服务端解 operation 的过程，确保我们构造的字节服务端能正确读出。
func TestAddUserOperationRoundTrip(t *testing.T) {
	account, err := BuildVlessAccount("uuid-789", vlessFlowVision)
	if err != nil {
		t.Fatalf("BuildVlessAccount: %v", err)
	}
	op := &handlerpb.AddUserOperation{
		User: &protocol.User{Email: "alice", Account: account},
	}
	opMsg, err := toTypedMessage(typeURLAddUserOp, op)
	if err != nil {
		t.Fatalf("toTypedMessage: %v", err)
	}
	if opMsg.Type != typeURLAddUserOp {
		t.Errorf("op type = %q, want %q", opMsg.Type, typeURLAddUserOp)
	}
	var decoded handlerpb.AddUserOperation
	if err := proto.Unmarshal(opMsg.Value, &decoded); err != nil {
		t.Fatalf("unmarshal op: %v", err)
	}
	if decoded.User.GetEmail() != "alice" {
		t.Errorf("email = %q, want alice", decoded.User.GetEmail())
	}
	if decoded.User.GetAccount().GetType() != typeURLVlessAccount {
		t.Errorf("account type = %q, want %q", decoded.User.GetAccount().GetType(), typeURLVlessAccount)
	}
}
