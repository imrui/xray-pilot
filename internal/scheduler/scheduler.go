package scheduler

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/service"
)

// Scheduler 管理所有定时任务
type Scheduler struct {
	syncSvc    *service.SyncService
	trafficSvc *service.TrafficService
	backupSvc  *service.BackupService
	settingSvc *service.SettingService
	nodeRepo   *repository.NodeRepository
	logRepo    *repository.LogRepository
	log        *zap.Logger
}

func New() *Scheduler {
	return &Scheduler{
		syncSvc:    service.NewSyncService(),
		trafficSvc: service.NewTrafficService(),
		backupSvc:  service.NewBackupService(),
		settingSvc: service.NewSettingService(),
		nodeRepo:   repository.NewNodeRepository(),
		logRepo:    repository.NewLogRepository(),
		log:        zap.L().Named("scheduler"),
	}
}

// Start 启动所有定时任务，ctx 取消时自动退出
func (s *Scheduler) Start(ctx context.Context) {
	driftInterval := s.settingSvc.GetInt(service.KeySchedulerDriftInterval)
	healthInterval := s.settingSvc.GetInt(service.KeySchedulerHealthInterval)
	trafficInterval := s.settingSvc.GetInt(service.KeySchedulerTrafficInterval)

	if driftInterval > 0 {
		go s.runLoop(ctx, "drift_check",
			time.Duration(driftInterval)*time.Second,
			0,
			s.runDriftCheck,
		)
	}

	if healthInterval > 0 {
		go s.runLoop(ctx, "health_check",
			time.Duration(healthInterval)*time.Second,
			15*time.Second,
			s.runHealthCheck,
		)
	}

	if trafficInterval > 0 {
		go s.runLoop(ctx, "traffic_poll",
			time.Duration(trafficInterval)*time.Second,
			30*time.Second, // 启动后 30s 延迟，避免与 health_check 抢资源
			func() { s.trafficSvc.RunOnce(ctx) },
		)
	}

	backupHours := s.settingSvc.GetInt(service.KeyBackupIntervalHours)
	if backupHours > 0 {
		go s.runLoop(ctx, "backup_run",
			time.Duration(backupHours)*time.Hour,
			1*time.Hour, // 启动后 1h 触发首次，避免启动风暴
			func() { s.backupSvc.RunOnce() },
		)
	}
}

// runLoop 通用定时循环，启动时不立即执行（等待第一个 tick），ctx 取消时退出
func (s *Scheduler) runLoop(ctx context.Context, name string, interval time.Duration, initialDelay time.Duration, task func()) {
	s.log.Info("定时任务已启动",
		zap.String("task", name),
		zap.Duration("interval", interval),
		zap.Duration("initial_delay", initialDelay),
	)

	if initialDelay > 0 {
		timer := time.NewTimer(initialDelay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			s.log.Info("定时任务已停止", zap.String("task", name))
			return
		case <-timer.C:
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.log.Info("定时任务已停止", zap.String("task", name))
			return
		case <-ticker.C:
			task()
		}
	}
}

// ---- 漂移检测 ----

func (s *Scheduler) runDriftCheck() {
	s.log.Debug("开始漂移检测")
	driftCount, errs := s.syncSvc.CheckDriftAll()
	if len(errs) > 0 {
		s.log.Warn("漂移检测部分失败",
			zap.Int("drift_count", driftCount),
			zap.Strings("errors", errs),
		)
	} else {
		s.log.Info("漂移检测完成", zap.Int("drift_count", driftCount))
	}
}

// ---- 健康检测 ----

func (s *Scheduler) runHealthCheck() {
	s.log.Debug("开始健康检测")
	nodes, err := s.nodeRepo.FindAll()
	if err != nil {
		s.log.Error("健康检测：查询节点失败", zap.Error(err))
		return
	}

	okCount, failCount := 0, 0
	for _, node := range nodes {
		node := node
		sshPort := node.SSHPort
		if sshPort == 0 {
			sshPort = 22
		}
		ok, latencyMs := tcpProbe(node.IP, sshPort)
		if err := s.nodeRepo.UpdateLastCheck(node.ID, ok, latencyMs); err != nil {
			s.log.Warn("健康检测：更新状态失败",
				zap.Uint("nodeID", node.ID),
				zap.Error(err),
			)
		}
		// 节点从健康变为不健康时，记录日志
		if !ok && node.LastCheckOK {
			s.logRepo.Record(
				"health_check",
				nodeTarget(node),
				false,
				"节点不可达（TCP 连接失败）",
				int64(latencyMs),
			)
			failCount++
		} else if ok {
			okCount++
		} else {
			failCount++
		}

	}
	s.log.Info("健康检测完成",
		zap.Int("ok", okCount),
		zap.Int("fail", failCount),
		zap.Int("total", len(nodes)),
	)
}

// tcpProbe 使用 TCP 连接探测节点可达性，返回 (ok, latencyMs)
func tcpProbe(ip string, port int) (ok bool, latencyMs int) {
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	elapsed := int(time.Since(start).Milliseconds())
	if err != nil {
		return false, elapsed
	}
	conn.Close()
	return true, elapsed
}

func nodeTarget(n entity.Node) string {
	return fmt.Sprintf("%s(%s)", n.Name, n.IP)
}
