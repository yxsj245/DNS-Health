// Package api Web API 层，路由定义与接口实现
package api

import (
	"dns-health-monitor/internal/connectivity"
	"time"

	"github.com/gin-gonic/gin"
)

// SetupRouter 配置并返回 Gin 路由引擎
func SetupRouter(
	authHandler *AuthHandler,
	credHandler *CredentialHandler,
	taskHandler *TaskHandler,
	statusHandler *StatusHandler,
	poolHandler *PoolHandler,
	notifHandler *NotificationHandler,
	jwtSecret []byte,
	fixedJWTSecret bool,
	startTime ...time.Time,
) *gin.Engine {
	return SetupRouterWithHealthMonitor(authHandler, credHandler, taskHandler, statusHandler, poolHandler, notifHandler, nil, nil, jwtSecret, fixedJWTSecret, startTime...)
}

// SetupRouterWithHealthMonitor 配置并返回 Gin 路由引擎（包含健康监控路由）
func SetupRouterWithHealthMonitor(
	authHandler *AuthHandler,
	credHandler *CredentialHandler,
	taskHandler *TaskHandler,
	statusHandler *StatusHandler,
	poolHandler *PoolHandler,
	notifHandler *NotificationHandler,
	healthMonitorHandler *HealthMonitorHandler,
	connectivityChecker *connectivity.Checker,
	jwtSecret []byte,
	fixedJWTSecret bool,
	startTime ...time.Time,
) *gin.Engine {
	// 创建SSE处理器
	sseHandler := NewSSEHandler()
	r := gin.Default()

	// API 路由组
	api := r.Group("/api")
	{
		// 公开接口（无需认证）
		api.GET("/setup-status", authHandler.CheckSetup) // 检查是否需要初始化注册
		api.POST("/register", authHandler.Register)      // 首次注册
		api.POST("/login", authHandler.Login)

		// 系统信息（公开，用于前端显示开发模式警告和运行时间）
		api.GET("/system-info", func(c *gin.Context) {
			resp := gin.H{
				"fixed_jwt_secret": fixedJWTSecret,
			}
			// 如果传入了启动时间，返回给前端用于计算运行时长
			if len(startTime) > 0 {
				resp["start_time"] = startTime[0].Format("2006-01-02T15:04:05Z07:00")
			}
			c.JSON(200, resp)
		})

		// 互联网连接状态（公开，用于前端显示连接状态）
		api.GET("/connectivity", func(c *gin.Context) {
			if connectivityChecker == nil {
				c.JSON(200, gin.H{"online": true, "enabled": false})
				return
			}
			status := connectivityChecker.GetStatus()
			c.JSON(200, gin.H{
				"enabled":               true,
				"online":                status.Online,
				"consecutive_fails":     status.ConsecutiveFails,
				"consecutive_successes": status.ConsecutiveSuccesses,
				"last_check_time":       status.LastCheckTime,
				"last_online_time":      status.LastOnlineTime,
				"down_since":            status.DownSince,
				"fail_threshold":        status.FailThreshold,
				"target":                status.Target,
			})
		})

		// 受保护接口（需要 JWT 认证）
		authorized := api.Group("")
		authorized.Use(JWTAuthMiddleware(jwtSecret))
		{
			// 登出
			authorized.POST("/logout", authHandler.Logout)

			// 账户管理
			authorized.GET("/account", authHandler.GetAccountInfo)
			authorized.PUT("/account/password", authHandler.ChangePassword)
			authorized.PUT("/account/username", authHandler.ChangeUsername)

			// 凭证管理
			authorized.GET("/credentials", credHandler.ListCredentials)
			authorized.POST("/credentials", credHandler.CreateCredential)
			authorized.PUT("/credentials/:id", credHandler.UpdateCredential)
			authorized.DELETE("/credentials/:id", credHandler.DeleteCredential)

			// 系统总览统计
			authorized.GET("/dashboard/stats", statusHandler.GetDashboardStats)

			// 任务健康状态（必须在 /tasks/:id 之前注册，避免 health 被当作 :id）
			authorized.GET("/tasks/health", statusHandler.GetTasksHealthStatus)

			// 探测任务 CRUD
			authorized.GET("/tasks", taskHandler.ListTasks)
			authorized.POST("/tasks", taskHandler.CreateTask)
			authorized.GET("/tasks/:id", taskHandler.GetTask)
			authorized.PUT("/tasks/:id", taskHandler.UpdateTask)
			authorized.DELETE("/tasks/:id", taskHandler.DeleteTask)
			authorized.POST("/tasks/:id/pause", taskHandler.PauseTask)                    // 暂停任务
			authorized.POST("/tasks/:id/resume", taskHandler.ResumeTask)                  // 恢复任务
			authorized.GET("/tasks/:id/switch-states", taskHandler.GetRecordSwitchStates) // 记录级别切换状态

			// 状态与历史查询
			authorized.GET("/tasks/:id/history", statusHandler.GetHistory)
			authorized.GET("/tasks/:id/history/stream", sseHandler.StreamTaskHistory) // SSE流式推送探测历史
			authorized.GET("/tasks/:id/logs", statusHandler.GetLogs)
			authorized.GET("/tasks/:id/logs/stream", sseHandler.StreamTaskLogs) // SSE流式推送操作日志
			authorized.GET("/tasks/:id/ips", statusHandler.GetTaskIPs)
			authorized.POST("/tasks/:id/ips/exclude", statusHandler.ExcludeIP)
			authorized.POST("/tasks/:id/ips/include", statusHandler.IncludeIP)
			authorized.GET("/tasks/:id/cname", statusHandler.GetCNAMEInfo)
			authorized.GET("/tasks/:id/latency", statusHandler.GetTaskLatency) // 延迟数据查询

			// 全局操作日志查询（支持按任务ID、操作类型、时间范围筛选）
			// 验证需求：10.3
			authorized.GET("/logs", statusHandler.GetAllLogs)

			// 统一系统日志（合并操作日志和通知记录）
			authorized.GET("/system-logs", statusHandler.GetSystemLogs)
			authorized.GET("/system-logs/stream", sseHandler.StreamSystemLogs) // SSE流式推送系统日志

			// 解析池管理
			authorized.POST("/pools", poolHandler.CreatePool)
			authorized.GET("/pools", poolHandler.ListPools)
			authorized.GET("/pools/:id", poolHandler.GetPool)
			authorized.PUT("/pools/:id", poolHandler.UpdatePool)
			authorized.GET("/pools/:id/health", poolHandler.GetPoolHealth)
			authorized.DELETE("/pools/:id", poolHandler.DeletePool)

			// 解析池资源管理（batch 路由必须在 /:resource_id 之前注册，避免 "batch" 被当作 resource_id）
			authorized.POST("/pools/:id/resources", poolHandler.AddResource)
			authorized.POST("/pools/:id/resources/batch", poolHandler.BatchAddResources)
			authorized.DELETE("/pools/:id/resources/:resource_id", poolHandler.RemoveResource)
			authorized.GET("/pools/:id/resources", poolHandler.ListResources)
			authorized.GET("/pools/:id/resources/:resource_id/resolve", poolHandler.ResolveDomainIPs)
			authorized.PUT("/pools/:id/resources/:resource_id/enable", poolHandler.EnableResource)
			authorized.PUT("/pools/:id/resources/:resource_id/disable", poolHandler.DisableResource)

			// 通知管理（SMTP 配置、任务通知设置、通知记录）
			if notifHandler != nil {
				notification := authorized.Group("/notification")
				{
					// SMTP 配置
					notification.GET("/smtp-config", notifHandler.GetSMTPConfig)
					notification.PUT("/smtp-config", notifHandler.SaveSMTPConfig)
					notification.POST("/smtp-test", notifHandler.TestSMTP)

					// 任务通知设置（batch 路由必须在 :taskId 之前注册，避免 "batch" 被当作 :taskId）
					notification.GET("/settings", notifHandler.GetNotificationSettings)
					notification.PUT("/settings/batch", notifHandler.BatchUpdateSettings)
					notification.PUT("/settings/:taskId", notifHandler.UpdateNotificationSetting)

					// 通知记录
					notification.GET("/logs", notifHandler.GetNotificationLogs)
				}
			}

			// 健康监控任务管理
			if healthMonitorHandler != nil {
				hm := authorized.Group("/health-monitors")
				{
					hm.POST("", healthMonitorHandler.CreateHealthMonitor)
					hm.GET("", healthMonitorHandler.ListHealthMonitors)
					hm.GET("/:id", healthMonitorHandler.GetHealthMonitor)
					hm.PUT("/:id", healthMonitorHandler.UpdateHealthMonitor)
					hm.POST("/:id/pause", healthMonitorHandler.PauseHealthMonitor)
					hm.POST("/:id/resume", healthMonitorHandler.ResumeHealthMonitor)
					hm.DELETE("/:id", healthMonitorHandler.DeleteHealthMonitor)
					hm.GET("/:id/results", healthMonitorHandler.GetHealthMonitorResults)
					hm.GET("/:id/results/stream", sseHandler.StreamHealthMonitorResults) // SSE流式推送探测结果
					hm.GET("/:id/latency", healthMonitorHandler.GetHealthMonitorLatency) // 延迟数据查询
				}
			}
		}
	}

	return r
}
