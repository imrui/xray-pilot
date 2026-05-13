package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/internal/xray"
	xssh "github.com/imrui/xray-pilot/pkg/ssh"
)

// TrafficService 节点流量统计服务
//
// 单次轮询语义（PollAll）：
//  1. 枚举所有 Active 且 LastCheckOK 的节点
//  2. 并发对每个节点 SSH 连接 → 通过 SSH 隧道访问其 127.0.0.1:10085 gRPC API
//  3. 调用 xray StatsService.QueryStats(pattern="user>>>", reset=true)
//  4. 解析增量、用 username→userID 映射转换、单事务写入明细 + 累加累计
//
// 失败处理：节点连接失败/查询失败时跳过本周期、不补 0、不影响其他节点
type TrafficService struct {
	nodeRepo    *repository.NodeRepository
	userRepo    *repository.UserRepository
	trafficRepo *repository.TrafficRepository
	logRepo     *repository.LogRepository
	settingSvc  *SettingService
}

func NewTrafficService() *TrafficService {
	return &TrafficService{
		nodeRepo:    repository.NewNodeRepository(),
		userRepo:    repository.NewUserRepository(),
		trafficRepo: repository.NewTrafficRepository(),
		logRepo:     repository.NewLogRepository(),
		settingSvc:  NewSettingService(),
	}
}

// PollResult 单次 PollAll 的汇总结果
type PollResult struct {
	PolledNodes    int
	FailedNodes    int
	WrittenSamples int
	UpdatedUsers   int
	Errors         []string
}

// PollAll 拉取所有活跃节点的流量增量并持久化
// 返回汇总结果与累计错误，调用方可决定是否记 log
func (s *TrafficService) PollAll(ctx context.Context) *PollResult {
	result := &PollResult{}

	nodes, err := s.nodeRepo.FindAll()
	if err != nil {
		result.Errors = append(result.Errors, "查询节点失败: "+err.Error())
		return result
	}

	users, err := s.userRepo.FindAll()
	if err != nil {
		result.Errors = append(result.Errors, "查询用户失败: "+err.Error())
		return result
	}
	emailToID := make(map[string]uint, len(users))
	for _, u := range users {
		if u.Username != "" {
			emailToID[u.Username] = u.ID
		}
	}

	// 收集所有节点的增量
	type nodeReport struct {
		nodeID  uint
		samples []entity.TrafficSample
		userSum map[uint][2]int64 // userID → [up, down]
		err     error
	}
	reports := make(chan nodeReport, len(nodes))
	periodEnd := time.Now()

	var wg sync.WaitGroup
	for _, node := range nodes {
		if !node.Active {
			continue
		}
		node := node
		wg.Add(1)
		go func() {
			defer wg.Done()
			samples, userSum, err := s.pollNode(ctx, &node, emailToID, periodEnd)
			reports <- nodeReport{nodeID: node.ID, samples: samples, userSum: userSum, err: err}
		}()
	}
	wg.Wait()
	close(reports)

	// 聚合：用户累计跨节点合并
	allSamples := make([]entity.TrafficSample, 0)
	userTotals := make(map[uint][2]int64)
	for r := range reports {
		result.PolledNodes++
		if r.err != nil {
			result.FailedNodes++
			result.Errors = append(result.Errors, fmt.Sprintf("node#%d: %v", r.nodeID, r.err))
			continue
		}
		allSamples = append(allSamples, r.samples...)
		for uid, deltas := range r.userSum {
			sum := userTotals[uid]
			sum[0] += deltas[0]
			sum[1] += deltas[1]
			userTotals[uid] = sum
		}
	}

	// 持久化
	totals := make([]entity.UserTrafficTotal, 0, len(userTotals))
	for uid, deltas := range userTotals {
		if deltas[0] == 0 && deltas[1] == 0 {
			continue
		}
		totals = append(totals, entity.UserTrafficTotal{
			UserID:        uid,
			UpBytes:       deltas[0],
			DownBytes:     deltas[1],
			LastUpdatedAt: periodEnd,
		})
	}

	if err := s.trafficRepo.WriteCycle(allSamples, totals); err != nil {
		result.Errors = append(result.Errors, "持久化失败: "+err.Error())
		return result
	}
	result.WrittenSamples = len(allSamples)
	result.UpdatedUsers = len(totals)
	return result
}

// pollNode 拉取单个节点的流量增量
//   - 建立 SSH 连接 → 通过 SSH 隧道 dial gRPC → QueryStats(reset=true) → 转换
//   - 用完即关闭 SSH 和 gRPC 连接（KISS：暂不做连接池，10 节点级规模性能足够）
func (s *TrafficService) pollNode(ctx context.Context, node *entity.Node, emailToID map[string]uint, periodEnd time.Time) ([]entity.TrafficSample, map[uint][2]int64, error) {
	sshCfg := s.sshConfig(node)
	sshClient, err := xssh.Connect(sshCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("ssh 连接失败: %w", err)
	}
	defer sshClient.Close()

	conn, err := sshClient.DialGRPC(ctx, "127.0.0.1:10085")
	if err != nil {
		return nil, nil, fmt.Errorf("gRPC 拨号失败: %w", err)
	}
	defer conn.Close()

	deltas, err := xray.PollUserTrafficDelta(ctx, conn)
	if err != nil {
		return nil, nil, fmt.Errorf("拉取流量失败: %w", err)
	}

	samples := make([]entity.TrafficSample, 0, len(deltas))
	userSum := make(map[uint][2]int64, len(deltas))
	for _, d := range deltas {
		uid, ok := emailToID[d.Email]
		if !ok {
			// 用户已删除但 xray 残留计数：丢弃。reset=true 已清零，下次不会再出现
			continue
		}
		if d.UpBytes == 0 && d.DownBytes == 0 {
			continue
		}
		samples = append(samples, entity.TrafficSample{
			UserID:    uid,
			NodeID:    node.ID,
			PeriodEnd: periodEnd,
			UpBytes:   d.UpBytes,
			DownBytes: d.DownBytes,
		})
		sum := userSum[uid]
		sum[0] += d.UpBytes
		sum[1] += d.DownBytes
		userSum[uid] = sum
	}
	return samples, userSum, nil
}

// sshConfig 从节点 + 系统默认值构建 SSH Config
// 注意：与 SyncService.sshParams 逻辑等价，未来若有第 3 处使用应抽公共函数
func (s *TrafficService) sshConfig(node *entity.Node) xssh.Config {
	port := node.SSHPort
	if port == 0 {
		port = s.settingSvc.GetInt(KeySSHDefaultPort)
		if port == 0 {
			port = 22
		}
	}
	user := node.SSHUser
	if user == "" {
		user = s.settingSvc.Get(KeySSHDefaultUser)
		if user == "" {
			user = "root"
		}
	}
	keyPath := node.SSHKeyPath
	if keyPath == "" {
		keyPath = s.settingSvc.Get(KeySSHDefaultKeyPath)
	}
	return xssh.Config{
		Host:           node.IP,
		Port:           port,
		User:           user,
		KeyPath:        keyPath,
		KnownHostsPath: s.settingSvc.Get(KeySSHKnownHostsPath),
	}
}

// GetSummary 仪表盘汇总指标（暴露给 handler）
func (s *TrafficService) GetSummary() (*repository.TrafficSummary, error) {
	return s.trafficRepo.Summary()
}

// GetTotalsForUsers 用户列表批量取累计（暴露给 handler）
func (s *TrafficService) GetTotalsForUsers(ids []uint) (map[uint]entity.UserTrafficTotal, error) {
	return s.trafficRepo.ListTotalsByUserIDs(ids)
}

// RunOnce 给 scheduler 用的封装：执行一次 PollAll，统一记日志
func (s *TrafficService) RunOnce(ctx context.Context) {
	start := time.Now()
	result := s.PollAll(ctx)
	zap.L().Named("traffic").Info("流量统计采集完成",
		zap.Int("polled", result.PolledNodes),
		zap.Int("failed", result.FailedNodes),
		zap.Int("samples", result.WrittenSamples),
		zap.Int("users", result.UpdatedUsers),
		zap.Duration("elapsed", time.Since(start)),
	)
	if len(result.Errors) > 0 {
		s.logRepo.Record("traffic_poll", "all", result.FailedNodes == 0, fmt.Sprintf("失败 %d/%d: %v", result.FailedNodes, result.PolledNodes, result.Errors), time.Since(start).Milliseconds())
	}
}
