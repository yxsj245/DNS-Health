// Package monitor 健康监控调度器
// 负责管理健康监控任务的定时调度，每个任务独立的定时器
// 支持动态启停和配置更新
package monitor

import (
	"context"
	"log"
	"sync"
	"time"

	"dns-health-monitor/internal/model"

	"gorm.io/gorm"
)

// monitorTaskRunner 单个健康监控任务的运行器
// 每个任务拥有独立的context和cancel函数，支持独立启停
type monitorTaskRunner struct {
	task   model.HealthMonitorTask // 任务配置
	cancel context.CancelFunc      // 取消函数，用于停止任务
}

// MonitorScheduler 健康监控调度器
// 管理所有健康监控任务的生命周期，包括启动、停止、更新
type MonitorScheduler struct {
	db       *gorm.DB                    // 数据库连接
	executor *MonitorExecutor            // 监控执行器
	tasks    map[uint]*monitorTaskRunner // taskID -> runner
	mu       sync.RWMutex                // 保护tasks的读写锁
	ctx      context.Context             // 父上下文
	cancel   context.CancelFunc          // 父上下文取消函数

	// 互联网连接检查器 - 用于在监控前检查互联网是否在线（可选）
	connectivityChecker ConnectivityChecker
}

// ConnectivityChecker 互联网连接状态检查接口
type ConnectivityChecker interface {
	IsOnline() bool
}

// NewMonitorScheduler 创建健康监控调度器实例
func NewMonitorScheduler(db *gorm.DB, executor *MonitorExecutor) *MonitorScheduler {
	return &MonitorScheduler{
		db:       db,
		executor: executor,
		tasks:    make(map[uint]*monitorTaskRunner),
	}
}

// Start 启动调度器，从数据库加载所有已启用的健康监控任务并启动
func (s *MonitorScheduler) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	var tasks []model.HealthMonitorTask
	if err := s.db.Where("enabled = ?", true).Find(&tasks).Error; err != nil {
		return err
	}

	log.Printf("健康监控调度器启动，加载了 %d 个监控任务", len(tasks))

	for _, task := range tasks {
		if err := s.startTask(task); err != nil {
			log.Printf("启动健康监控任务 %d (%s.%s) 失败: %v", task.ID, task.SubDomain, task.Domain, err)
			continue
		}
	}

	return nil
}

// Stop 停止调度器，取消所有运行中的任务
func (s *MonitorScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, runner := range s.tasks {
		runner.cancel()
		log.Printf("已停止健康监控任务 %d", id)
	}
	s.tasks = make(map[uint]*monitorTaskRunner)

	if s.cancel != nil {
		s.cancel()
	}

	log.Println("健康监控调度器已停止")
}

// AddTask 添加并启动新的健康监控任务
// 需求 1.10: 创建任务成功后自动启动该监控任务
func (s *MonitorScheduler) AddTask(task model.HealthMonitorTask) error {
	s.mu.Lock()
	// 如果任务已存在，先停止旧的
	if runner, exists := s.tasks[task.ID]; exists {
		runner.cancel()
		delete(s.tasks, task.ID)
	}
	s.mu.Unlock()

	return s.startTask(task)
}

// StopTask 停止指定的健康监控任务
// 需求 7.1: 暂停任务时停止该任务的DNS解析和探测
func (s *MonitorScheduler) StopTask(taskID uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if runner, exists := s.tasks[taskID]; exists {
		runner.cancel()
		delete(s.tasks, taskID)
		log.Printf("已停止健康监控任务 %d", taskID)
	}

	return nil
}

// RestartTask 使用新配置重新启动任务
// 需求 7.2: 恢复任务时重新启动该任务的DNS解析和探测
// 需求 7.5: 更新任务配置时使用新配置重新启动监控
func (s *MonitorScheduler) RestartTask(task model.HealthMonitorTask) error {
	// 先停止旧任务
	s.mu.Lock()
	if runner, exists := s.tasks[task.ID]; exists {
		runner.cancel()
		delete(s.tasks, task.ID)
	}
	s.mu.Unlock()

	// 只有启用状态的任务才重新启动
	if !task.Enabled {
		log.Printf("健康监控任务 %d 已禁用，不启动", task.ID)
		return nil
	}

	return s.startTask(task)
}

// RemoveTask 移除健康监控任务（停止并清理）
// 需求 7.3: 停止任务并删除所有相关数据
func (s *MonitorScheduler) RemoveTask(taskID uint) error {
	return s.StopTask(taskID)
}

// IsRunning 检查指定任务是否正在运行
func (s *MonitorScheduler) IsRunning(taskID uint) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, exists := s.tasks[taskID]
	return exists
}

// RunningTaskCount 返回当前运行中的任务数量
func (s *MonitorScheduler) RunningTaskCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.tasks)
}

// SetConnectivityChecker 设置互联网连接检查器
func (s *MonitorScheduler) SetConnectivityChecker(checker ConnectivityChecker) {
	s.connectivityChecker = checker
}

// startTask 启动单个健康监控任务的定时执行goroutine
func (s *MonitorScheduler) startTask(task model.HealthMonitorTask) error {
	if s.ctx == nil {
		return nil
	}

	// 创建任务独立的上下文
	taskCtx, cancel := context.WithCancel(s.ctx)

	runner := &monitorTaskRunner{
		task:   task,
		cancel: cancel,
	}

	s.mu.Lock()
	s.tasks[task.ID] = runner
	s.mu.Unlock()

	// 启动定时执行goroutine
	go s.runMonitorLoop(taskCtx, runner)

	log.Printf("已启动健康监控任务 %d: %s.%s (协议=%s, 周期=%ds)",
		task.ID, task.SubDomain, task.Domain, task.ProbeProtocol, task.ProbeIntervalSec)

	return nil
}

// runMonitorLoop 单个健康监控任务的定时执行循环
// 根据任务的ProbeIntervalSec配置定时执行监控
func (s *MonitorScheduler) runMonitorLoop(ctx context.Context, runner *monitorTaskRunner) {
	task := runner.task
	interval := time.Duration(task.ProbeIntervalSec) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 立即执行一次监控
	if s.connectivityChecker == nil || s.connectivityChecker.IsOnline() {
		s.executeMonitor(ctx, &task)
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("健康监控任务 %d 已停止", task.ID)
			return
		case <-ticker.C:
			// 检查互联网连接状态，断网时跳过监控
			if s.connectivityChecker != nil && !s.connectivityChecker.IsOnline() {
				continue
			}
			s.executeMonitor(ctx, &task)
		}
	}
}

// executeMonitor 执行一次监控周期
func (s *MonitorScheduler) executeMonitor(ctx context.Context, task *model.HealthMonitorTask) {
	// 从数据库重新加载最新的任务配置
	var latestTask model.HealthMonitorTask
	if err := s.db.First(&latestTask, task.ID).Error; err != nil {
		log.Printf("健康监控任务 %d: 加载任务配置失败: %v", task.ID, err)
		return
	}

	// 如果任务已被禁用，跳过本次执行
	if !latestTask.Enabled {
		return
	}

	// 调用执行器执行一次完整的监控周期
	if err := s.executor.Execute(ctx, &latestTask); err != nil {
		log.Printf("健康监控任务 %d: 执行失败: %v", task.ID, err)
	}
}
