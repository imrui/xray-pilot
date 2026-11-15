package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

// LiveSyncService 基于当前授权快照进行持久化与 gRPC 用户差异同步。
//
// 定位：用户增删时，对相关节点的 xray 走 gRPC HandlerService.AlterInbound 即时增删用户，
// 不重启 xray，消除「加一个用户 → 全节点 restart → 断所有现有连接」的抖动。
//
// 双写语义（关键）：
//   - 预检后原子持久化当前 config.json（不重启），保持重启后的权限一致
//   - gRPC AddUser/RemoveUser —— 即时运行时生效
//   - 任一步骤失败均标记未完成，由持久化恢复队列限次重试；不忽略单 inbound 错误
//
// 受 xray.live_apply_enabled 开关控制（默认 off）。
type LiveSyncService struct {
	syncSvc    *SyncService // 复用 buildConfig / sshParams（同包可访问私有方法）
	nodeRepo   *repository.NodeRepository
	settingSvc *SettingService
}

func NewLiveSyncService() *LiveSyncService {
	return &LiveSyncService{
		syncSvc:    NewSyncService(),
		nodeRepo:   repository.NewNodeRepository(),
		settingSvc: NewSettingService(),
	}
}

// Enabled 返回 live-apply 开关是否打开。off 时调用方应回退到 nodeRepo.MarkAllDrifted()。
func (s *LiveSyncService) Enabled() bool {
	return s.settingSvc.Get(KeyXrayLiveApply) == "true"
}

// AddUserLive 根据数据库当前授权同步，避免重放过期的 UUID 参数。
func (s *LiveSyncService) AddUserLive(nodeIDs []uint, email, uuid string) {
	s.applyOverNodes(nodeIDs, "新增用户", email)
}

func (s *LiveSyncService) RemoveUserLive(nodeIDs []uint, email string) {
	s.applyOverNodes(nodeIDs, "摘除用户", email)
}

func (s *LiveSyncService) ReplaceUserLive(nodeIDs []uint, email, newUUID string) {
	s.applyOverNodes(nodeIDs, "重置用户UUID", email)
}

// applyOverNodes 每个节点独立处理。数据库修改已提交，失败交由恢复队列处理。
func (s *LiveSyncService) applyOverNodes(nodeIDs []uint, action, email string) {
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
		if err := s.applyOnNode(node); err != nil {
			failed++
			zap.L().Warn("live-apply 失败，已标记节点漂移待 restart 同步",
				zap.String("action", action), zap.String("node", node.Name),
				zap.Uint("nodeID", node.ID), zap.String("email", email), zap.Error(err))
			_ = s.nodeRepo.UpdateSyncStatus(node.ID, entity.SyncStatusDrifted, "")
			recordConfigFailure(node.ID, err.Error())
		} else {
			ok++
		}
	}
	zap.L().Info("live-apply 完成",
		zap.String("action", action), zap.String("email", email),
		zap.Int("ok", ok), zap.Int("failed", failed))
}

// applyOnNode 先持久化当前配置，再根据已确认运行快照对齐各 inbound 用户。
// 任一环节失败都不确认运行态成功。
func (s *LiveSyncService) applyOnNode(node *entity.Node) error {
	defer lockNode(node.ID)()
	state, release, err := repository.AcquireConfigLease(node.ID)
	if err != nil {
		return err
	}
	defer release()
	node, err = s.nodeRepo.FindByID(node.ID)
	if err != nil {
		return err
	}

	// 1. 生成最新 config（buildConfig 只含激活用户，反映增删后的最终状态）
	configContent, _, err := s.syncSvc.buildConfig(node)
	if err != nil {
		return fmt.Errorf("生成配置失败: %w", err)
	}

	// 2. SSH 连接（一条连接复用于配置持久化和 gRPC 隧道）
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
	old, err := client.ReadRemoteFile(params.ConfigPath)
	if err != nil {
		return fmt.Errorf("无法读取节点配置")
	}
	if state.DesiredRevision > 0 && state.RuntimeHash != xray.ConfigHash(old) {
		return fmt.Errorf("上次运行版本尚未确认，需完整同步")
	}
	// Live apply is valid only for a pure user delta against the last confirmed runtime.
	if node.ConfigHash == "" || xray.ConfigHash(old) != node.ConfigHash || !xray.SameNonUserConfig(old, configContent) {
		return fmt.Errorf("存在非用户配置或未确认磁盘版本，需完整同步")
	}
	if err := rememberDesired(state, xray.ConfigHash(configContent)); err != nil {
		return err
	}

	// 3. scp 最新配置（不重启）—— 持久化，重启可恢复
	if err := client.ApplyXrayConfig(configContent, params.ConfigPath, false, false, xssh.TransactionOptions{BaselineHash: node.ConfigHash, ProbeURL: params.ProbeURL, BeforeApply: func() error {
		if !s.syncSvc.stillCurrent(state, xray.ConfigHash(configContent)) {
			return fmt.Errorf("期望配置已更新")
		}
		return nil
	}}); err != nil {
		return fmt.Errorf("上传配置失败: %w", err)
	}
	if err := repository.DB.Model(&entity.NodeConfigState{}).Where("node_id = ? AND lease_owner = ?", node.ID, state.LeaseOwner).Updates(map[string]any{"disk_revision": state.DesiredRevision, "disk_hash": xray.ConfigHash(configContent)}).Error; err != nil {
		return err
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

	changes, err := xray.DiffUsers(old, configContent)
	if err != nil {
		return fmt.Errorf("查询节点协议失败: %w", err)
	}
	for _, change := range changes {
		if change.Remove {
			err = xray.RemoveUser(ctx, conn, change.Tag, change.Email)
		} else {
			account, accountErr := xray.BuildAccountForProtocol(change.Protocol, change.UUID)
			if accountErr != nil {
				return accountErr
			}
			err = xray.AddUser(ctx, conn, change.Tag, change.Email, account)
		}
		if err != nil {
			return fmt.Errorf("用户运行态尚未确认（inbound %s），需完整同步", change.Tag)
		}
	}

	// 5. 仅当节点此前已 synced 时才回写 synced。
	// 若此前是 drifted/pending（仍有走 MarkAllDrifted 的待同步变更，如改协议/分组/UUID，
	// 这些不是单纯加减用户、gRPC 无法即时对齐），保留原状态让 restart 同步路径补齐——
	// 否则清掉 drift 会导致那些待同步变更运行时缺失且无人修复。本次用户增删的即时效果已生效。
	if s.syncSvc.stillCurrent(state, xray.ConfigHash(configContent)) {
		if err := s.syncSvc.recordApplied(state, configContent, state.ProcessID, nil); err != nil {
			return err
		}
		if err := finishNodeStatus(state, xray.ConfigHash(configContent), true); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("期望配置已更新，撤销尚未确认")
	}
	return nil
}
