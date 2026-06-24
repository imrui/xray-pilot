package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

// LiveSyncService 用户增删的 gRPC 即时生效编排（v0.4.5）。
//
// 定位：用户增删时，对相关节点的 xray 走 gRPC HandlerService.AlterInbound 即时增删用户，
// 不重启 xray，消除「加一个用户 → 全节点 restart → 断所有现有连接」的抖动。
//
// 双写语义（关键）：
//   - scp 最新 config.json（不重启）—— 持久化，重启可恢复，是正确性保证
//   - gRPC AddUser/RemoveUser —— 即时运行时生效
//   - 任一节点的 SSH/scp/dial 失败 → 该节点标记 drifted，回退到现有 restart 同步路径兜底
//
// 受 xray.live_apply_enabled 开关控制（默认 off）。
type LiveSyncService struct {
	syncSvc     *SyncService // 复用 buildConfig / sshParams（同包可访问私有方法）
	nodeRepo    *repository.NodeRepository
	profileRepo *repository.InboundProfileRepository
	settingSvc  *SettingService
}

func NewLiveSyncService() *LiveSyncService {
	return &LiveSyncService{
		syncSvc:     NewSyncService(),
		nodeRepo:    repository.NewNodeRepository(),
		profileRepo: repository.NewInboundProfileRepository(),
		settingSvc:  NewSettingService(),
	}
}

// Enabled 返回 live-apply 开关是否打开。off 时调用方应回退到 nodeRepo.MarkAllDrifted()。
func (s *LiveSyncService) Enabled() bool {
	return s.settingSvc.Get(KeyXrayLiveApply) == "true"
}

// inboundFn 对单个 inbound（按 tag 定位）执行的操作（AddUser / RemoveUser）。
// profile 提供协议类型，供 add 侧构造账号。返回错误时被容忍（仅 warn）——
// 因为 scp 已保证磁盘配置正确，单 inbound 的 gRPC 失败最常见是「用户已存在/本就不在」，end-state 正确。
type inboundFn func(ctx context.Context, conn *grpc.ClientConn, tag string, profile *entity.InboundProfile) error

// AddUserLive 对给定节点集即时新增用户（按协议构造账号，email/uuid 来自用户）。
func (s *LiveSyncService) AddUserLive(nodeIDs []uint, email, uuid string) {
	s.applyOverNodes(nodeIDs, "新增用户", email, func(ctx context.Context, conn *grpc.ClientConn, tag string, profile *entity.InboundProfile) error {
		account, err := xray.BuildAccountForProtocol(profile.Protocol, uuid)
		if err != nil {
			return err // 不可管理协议已被 InboundTag 过滤，正常不会走到这
		}
		return xray.AddUser(ctx, conn, tag, email, account)
	})
}

// RemoveUserLive 对给定节点集即时摘除用户（按 email 定位）。
func (s *LiveSyncService) RemoveUserLive(nodeIDs []uint, email string) {
	s.applyOverNodes(nodeIDs, "摘除用户", email, func(ctx context.Context, conn *grpc.ClientConn, tag string, _ *entity.InboundProfile) error {
		return xray.RemoveUser(ctx, conn, tag, email)
	})
}

// ReplaceUserLive 对给定节点集即时替换用户账号（email 不变、UUID 变，用于 ResetUUID）。
// 每 inbound 先删旧账号（按 email；不存在也无妨）再加新 UUID 账号。
func (s *LiveSyncService) ReplaceUserLive(nodeIDs []uint, email, newUUID string) {
	s.applyOverNodes(nodeIDs, "重置用户UUID", email, func(ctx context.Context, conn *grpc.ClientConn, tag string, profile *entity.InboundProfile) error {
		_ = xray.RemoveUser(ctx, conn, tag, email) // 旧账号；按 email 摘除，不存在不报致命
		account, err := xray.BuildAccountForProtocol(profile.Protocol, newUUID)
		if err != nil {
			return err
		}
		return xray.AddUser(ctx, conn, tag, email, account)
	})
}

// applyOverNodes 在节点集上扇出执行 perInbound：每节点独立处理，失败标记 drifted 兜底。
// 不返回错误——失败已就地降级，不阻断上层用户操作（DB 已写入为准）。
func (s *LiveSyncService) applyOverNodes(nodeIDs []uint, action, email string, perInbound inboundFn) {
	if len(nodeIDs) == 0 {
		return
	}
	nodes, err := s.nodeRepo.FindByIDs(nodeIDs)
	if err != nil {
		zap.L().Warn("live-apply 查询节点失败，回退标记漂移", zap.String("action", action), zap.Error(err))
		_ = s.nodeRepo.MarkAllDrifted()
		return
	}
	ok, failed := 0, 0
	for i := range nodes {
		node := &nodes[i]
		if err := s.applyOnNode(node, perInbound); err != nil {
			failed++
			zap.L().Warn("live-apply 失败，已标记节点漂移待 restart 同步",
				zap.String("action", action), zap.String("node", node.Name),
				zap.Uint("nodeID", node.ID), zap.String("email", email), zap.Error(err))
			_ = s.nodeRepo.UpdateSyncStatus(node.ID, entity.SyncStatusDrifted, "")
		} else {
			ok++
		}
	}
	zap.L().Info("live-apply 完成",
		zap.String("action", action), zap.String("email", email),
		zap.Int("ok", ok), zap.Int("failed", failed))
}

// applyOnNode 单节点脚手架：scp 最新 config（不重启）+ 逐 inbound 经 gRPC 执行 perInbound。
// 返回非 nil（SSH/scp/dial/配置生成失败）时，调用方将该节点标记 drifted 兜底。
func (s *LiveSyncService) applyOnNode(node *entity.Node, perInbound inboundFn) error {
	// 1. 生成最新 config（buildConfig 只含激活用户，反映增删后的最终状态）
	configContent, _, err := s.syncSvc.buildConfig(node)
	if err != nil {
		return fmt.Errorf("生成配置失败: %w", err)
	}

	// 2. SSH 连接（一条连接复用于 scp + gRPC 隧道）
	params := s.syncSvc.sshParams(node)
	client, err := xssh.Connect(xssh.Config{
		Host:           params.Host,
		Port:           params.Port,
		User:           params.User,
		KeyPath:        params.KeyPath,
		KnownHostsPath: params.KnownHostsPath,
	})
	if err != nil {
		return fmt.Errorf("SSH 连接失败: %w", err)
	}
	defer client.Close()

	// 3. scp 最新配置（不重启）—— 持久化，重启可恢复
	if err := client.UploadContent(configContent, xray.RemoteConfigPath); err != nil {
		return fmt.Errorf("上传配置失败: %w", err)
	}

	// 4. gRPC 逐 inbound 即时生效
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := client.DialGRPC(ctx, xray.GRPCAPIAddress)
	if err != nil {
		// scp 已成功（磁盘正确），但运行时未即时变更 → 标记 drifted 强制后续 restart 对齐
		return fmt.Errorf("gRPC 拨号失败: %w", err)
	}
	defer conn.Close()

	keys, err := s.profileRepo.FindActiveKeysForNode(node.ID)
	if err != nil {
		return fmt.Errorf("查询节点协议失败: %w", err)
	}
	for _, key := range keys {
		if key.Profile == nil {
			continue
		}
		tag, manageable := xray.InboundTag(key.Profile.Protocol, key.Profile.ID)
		if !manageable {
			continue // Hysteria2 不在 xray-core，跳过
		}
		if err := perInbound(ctx, conn, tag, key.Profile); err != nil {
			// 容忍：scp 已保证磁盘正确；单 inbound 失败最常见是「用户已存在/本就不在」，end-state 正确
			zap.L().Warn("gRPC 单 inbound 操作失败（已容忍，磁盘配置已正确）",
				zap.String("node", node.Name), zap.String("tag", tag), zap.Error(err))
		}
	}

	// 5. 仅当节点此前已 synced 时才回写 synced。
	// 若此前是 drifted/pending（仍有走 MarkAllDrifted 的待同步变更，如改协议/分组/UUID，
	// 这些不是单纯加减用户、gRPC 无法即时对齐），保留原状态让 restart 同步路径补齐——
	// 否则清掉 drift 会导致那些待同步变更运行时缺失且无人修复。本次用户增删的即时效果已生效。
	if node.SyncStatus == entity.SyncStatusSynced {
		if err := s.nodeRepo.UpdateLastSync(node.ID, entity.SyncStatusSynced, xray.ConfigHash(configContent)); err != nil {
			zap.L().Warn("更新同步状态失败", zap.Uint("nodeID", node.ID), zap.Error(err))
		}
	}
	return nil
}
