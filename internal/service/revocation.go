package service

import (
	"fmt"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
	"time"
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

// 只有全部相关节点已对齐当前权限，或确认停止服务，才确认过期摘除。
func (s *UserService) confirmRevoked(ids []uint) bool {
	all := true
	for _, id := range ids {
		if !confirmNodeRevocation(id) {
			all = false
		}
	}
	return all
}
func confirmNodeRevocation(id uint) bool {
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
	if err := stopRevokedNode(svc.sshParams(n)); err != nil {
		_ = leaseUpdate(repository.DB, state, map[string]any{"recovery_pending": true, "last_error": "撤权未确认，节点不可达或停止失败", "runtime_hash": "", "runtime_authorization_hash": "", "verified_at": nil})
		svc.reportSyncAlert(id, false, "撤权未确认：节点不可达或停止失败")
		return false
	}
	msg := "AUTHORIZATION_STOPPED：已停止节点，等待当前授权配置恢复"
	_ = leaseUpdate(repository.DB, state, map[string]any{"recovery_pending": true, "runtime_hash": "", "runtime_authorization_hash": "", "verified_at": nil, "last_error": msg, "next_retry_at": time.Now().Add(time.Minute)})
	_ = svc.nodeRepo.UpdateSyncStatus(id, entity.SyncStatusDrifted, "")
	repository.NewLogRepository().RecordWithActor("authorization_stop", fmt.Sprintf("node:%d", id), "system:expiry", false, msg, 0)
	svc.reportSyncAlert(id, false, msg)
	return true
}
