// Package connectivity 互联网连接状态检测模块
// 通过持续 ping 目标地址（默认 www.baidu.com）来检测互联网连接状态。
// 连续丢包达到阈值时判定为断网，并通知调度器暂停所有探测任务。
package connectivity

import (
	"context"
	"log"
	"net"
	"sync"
	"time"
)

// 默认配置常量
const (
	// DefaultTarget 默认 ping 目标
	DefaultTarget = "www.baidu.com"
	// DefaultPort 默认 TCP 探测端口
	DefaultPort = "80"
	// DefaultInterval 默认探测间隔
	DefaultInterval = 3 * time.Second
	// DefaultTimeout 默认超时时间
	DefaultTimeout = 3 * time.Second
	// DefaultFailThreshold 连续失败阈值，达到此值判定为断网
	DefaultFailThreshold = 10
	// DefaultRecoverThreshold 连续成功阈值，达到此值判定为恢复
	DefaultRecoverThreshold = 3
)

// Status 互联网连接状态
type Status struct {
	// Online 是否在线
	Online bool `json:"online"`
	// ConsecutiveFails 当前连续失败次数
	ConsecutiveFails int `json:"consecutive_fails"`
	// ConsecutiveSuccesses 当前连续成功次数
	ConsecutiveSuccesses int `json:"consecutive_successes"`
	// LastCheckTime 最后一次检测时间
	LastCheckTime time.Time `json:"last_check_time"`
	// LastOnlineTime 最后一次在线时间
	LastOnlineTime time.Time `json:"last_online_time"`
	// DownSince 断网开始时间（在线时为零值）
	DownSince time.Time `json:"down_since,omitempty"`
	// FailThreshold 失败阈值
	FailThreshold int `json:"fail_threshold"`
	// Target 探测目标
	Target string `json:"target"`
}

// PauseResumeCallback 暂停/恢复回调函数类型
// pause=true 表示暂停所有任务，pause=false 表示恢复所有任务
type PauseResumeCallback func(pause bool)

// Checker 互联网连接检查器
type Checker struct {
	target           string
	port             string
	interval         time.Duration
	timeout          time.Duration
	failThreshold    int
	recoverThreshold int

	mu               sync.RWMutex
	online           bool
	consecutiveFails int
	consecutiveSuccs int
	lastCheckTime    time.Time
	lastOnlineTime   time.Time
	downSince        time.Time

	callbacks []PauseResumeCallback
}

// NewChecker 创建互联网连接检查器
func NewChecker() *Checker {
	return &Checker{
		target:           DefaultTarget,
		port:             DefaultPort,
		interval:         DefaultInterval,
		timeout:          DefaultTimeout,
		failThreshold:    DefaultFailThreshold,
		recoverThreshold: DefaultRecoverThreshold,
		online:           true, // 初始假设在线
		lastOnlineTime:   time.Now(),
	}
}

// OnPauseResume 注册暂停/恢复回调
func (c *Checker) OnPauseResume(cb PauseResumeCallback) {
	c.callbacks = append(c.callbacks, cb)
}

// IsOnline 返回当前互联网是否在线
func (c *Checker) IsOnline() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.online
}

// GetStatus 获取当前连接状态详情
func (c *Checker) GetStatus() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return Status{
		Online:               c.online,
		ConsecutiveFails:     c.consecutiveFails,
		ConsecutiveSuccesses: c.consecutiveSuccs,
		LastCheckTime:        c.lastCheckTime,
		LastOnlineTime:       c.lastOnlineTime,
		DownSince:            c.downSince,
		FailThreshold:        c.failThreshold,
		Target:               c.target,
	}
}

// Start 启动连接检查器，持续检测互联网连接状态
func (c *Checker) Start(ctx context.Context) {
	log.Printf("互联网连接检查器已启动，目标: %s:%s，间隔: %v，失败阈值: %d",
		c.target, c.port, c.interval, c.failThreshold)

	go c.runLoop(ctx)
}

// runLoop 检测循环
func (c *Checker) runLoop(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// 立即执行一次检测
	c.check()

	for {
		select {
		case <-ctx.Done():
			log.Println("互联网连接检查器已停止")
			return
		case <-ticker.C:
			c.check()
		}
	}
}

// check 执行一次连接检测（使用 TCP 连接代替 ICMP ping，无需特权）
func (c *Checker) check() {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(c.target, c.port), c.timeout)
	success := err == nil
	if conn != nil {
		conn.Close()
	}

	c.mu.Lock()
	now := time.Now()
	c.lastCheckTime = now
	wasOnline := c.online

	if success {
		c.consecutiveFails = 0
		c.consecutiveSuccs++
		c.lastOnlineTime = now

		// 如果之前是断网状态，检查是否达到恢复阈值
		if !c.online && c.consecutiveSuccs >= c.recoverThreshold {
			c.online = true
			c.downSince = time.Time{}
			log.Printf("互联网连接已恢复（连续成功 %d 次）", c.consecutiveSuccs)
		}
	} else {
		c.consecutiveSuccs = 0
		c.consecutiveFails++

		// 如果之前是在线状态，检查是否达到失败阈值
		if c.online && c.consecutiveFails >= c.failThreshold {
			c.online = false
			c.downSince = now
			log.Printf("互联网连接已断开（连续失败 %d 次），暂停所有探测任务", c.consecutiveFails)
		}
	}
	c.mu.Unlock()

	// 状态变化时触发回调
	if wasOnline != c.IsOnline() {
		pause := !c.IsOnline()
		for _, cb := range c.callbacks {
			cb(pause)
		}
	}
}
