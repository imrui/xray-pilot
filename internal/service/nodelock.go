package service

import "sync"

// nodeLocks 节点级互斥：同一节点的配置推送（SyncNode / live-apply）串行执行。
//
// 远端经固定的 config.json.tmp 中转再 mv，同一节点并发推送会互相 mv 掉对方的临时文件，
// 表现为「移动临时文件失败」（v0.5.1 实测：节点页「同步待处理」与顶栏「待同步」同时触发）。
// 单进程面板，进程内锁足够；不同节点之间互不阻塞。
var nodeLocks sync.Map // map[uint]*sync.Mutex

// lockNode 获取节点锁，返回解锁函数：defer lockNode(id)()
func lockNode(id uint) func() {
	v, _ := nodeLocks.LoadOrStore(id, &sync.Mutex{})
	m := v.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}
