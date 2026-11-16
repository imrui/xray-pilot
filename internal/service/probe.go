package service

import (
	"fmt"
	"net"
	"time"
)

// TCPProbe 对 ip:port 做一次 TCP 握手探针，返回是否可达与耗时（毫秒）。
//
// 5 秒超时，与既有 health_check 调度任务保持一致。
// 该 helper 同时被 scheduler 健康检测和 install register 注册后连通性自检使用。
func TCPProbe(ip string, port int) (ok bool, latencyMs int) {
	latencyMs, err := tcpProbe(ip, port)
	return err == nil, latencyMs
}

// tcpProbe 保留底层错误，供健康检测写入日志和告警。
func tcpProbe(ip string, port int) (latencyMs int, err error) {
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	elapsed := int(time.Since(start).Milliseconds())
	if err != nil {
		return elapsed, err
	}
	_ = conn.Close()
	return elapsed, nil
}
