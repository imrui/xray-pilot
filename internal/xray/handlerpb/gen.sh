#!/usr/bin/env bash
# 重生成 proxyman HandlerService 的 vendored protobuf。
# 依赖：protoc + protoc-gen-go + protoc-gen-go-grpc（后两者在 $(go env GOPATH)/bin）。
# 在本目录（internal/xray/handlerpb）下执行：bash gen.sh
set -euo pipefail
cd "$(dirname "$0")"
export PATH="$(go env GOPATH)/bin:$PATH"

protoc -I . \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  handler.proto \
  common/serial/typed_message.proto \
  common/protocol/user.proto \
  proxy/vless/account.proto \
  proxy/trojan/account.proto

echo "✅ handlerpb 生成完成"
