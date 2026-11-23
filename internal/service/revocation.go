package service

import (
	"fmt"
	"time"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
	"gorm.io/gorm"
)

var stopRevokedNode = func(p xray.SSHParams) error {
	c, err := xssh.Connect(xssh.Config{Host: p.Host, Port: p.Port, User: p.User, KeyPath: p.KeyPath, KnownHostsPath: p.KnownHostsPath})
	if err != nil {
		return err
	}
	defer c.Close()
	_, err = c.RunBounded("systemctl stop xray && test \"$(systemctl show xray -p MainPID --value)\" = 0", 15*time.Second)
	return err
}

// 租约冲突或离线时保留待办，续期已取消的旧任务不能触发停止保护。
func confirmNodeRevocation(id uint, tickets []string) bool {
	r := NewSyncService().SyncNode(id, "system:expiry")
	if r.Success {
		return true
	}
	if r.Busy {
		return false
	}
	defer lockNode(id)()
	state, release, err := repository.AcquireConfigLease(id)
	if err != nil {
		return false
	}
	defer release()
	svc := NewSyncService()
	n, err := svc.nodeRepo.FindByID(id)
	if err != nil {
		return false
	}
	raw, _, buildErr := svc.buildConfig(n)
	if buildErr == nil && state.VerifiedAt != nil && state.RuntimeHash != "" && state.DiskHash == state.RuntimeHash {
		// Recovery may run the previous routing policy with current credentials.
		// A confirmed authorization receipt suffices for revocation; the policy
		// itself stays pending and is retried on its normal recovery schedule.
		if state.RuntimeHash == xray.ConfigHash(raw) || state.RuntimeAuthorizationHash != "" && state.RuntimeAuthorizationHash == xssh.AuthorizationHash(raw) {
			return true
		}
	}
	// 无法证明撤权已生效时，在线节点停止服务；离线节点保留待处理，不标为已摘除。
	stillPending := false
	for len(tickets) > 0 && !stillPending {
		n := min(500, len(tickets))
		var count int64
		if err := repository.DB.Model(&entity.UserExpiryTarget{}).Where("node_id = ? AND ticket IN ?", id, tickets[:n]).Count(&count).Error; err != nil {
			return false
		}
		stillPending = count != 0
		tickets = tickets[n:]
	}
	if !stillPending {
		return false
	}
	if err := stopRevokedNode(svc.sshParams(n)); err != nil {
		_ = leaseUpdate(repository.DB, state, map[string]any{"recovery_pending": true, "last_error": "撤权未确认，节点不可达或停止失败", "runtime_hash": "", "runtime_authorization_hash": "", "verified_at": nil})
		svc.reportSyncAlert(state, false, "撤权未确认：节点不可达或停止失败")
		return false
	}
	msg := "AUTHORIZATION_STOPPED：已停止节点，等待当前授权配置恢复"
	if err := repository.DB.Transaction(func(tx *gorm.DB) error {
		updated := tx.Model(&entity.Node{}).Where("id = ?", id).Update("sync_status", entity.SyncStatusDrifted)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return leaseUpdate(tx, state, map[string]any{
			"recovery_pending": true, "runtime_hash": "", "runtime_authorization_hash": "", "verified_at": nil,
			"last_error": msg, "next_retry_at": time.Now().Add(time.Minute),
		})
	}); err != nil {
		return false
	}
	repository.NewLogRepository().RecordWithActor("authorization_stop", fmt.Sprintf("node:%d", id), "system:expiry", false, msg, 0)
	svc.reportSyncAlert(state, false, msg)
	return true
}
