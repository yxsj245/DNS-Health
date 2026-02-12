// Package api SSE（Server-Sent Events）流式推送接口
// 为任务详情、健康监控详情、系统日志等页面提供实时数据推送
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"

	"dns-health-monitor/internal/sse"

	"github.com/gin-gonic/gin"
)

// SSEHandler SSE流式推送处理器
type SSEHandler struct{}

// NewSSEHandler 创建SSE处理器
func NewSSEHandler() *SSEHandler {
	return &SSEHandler{}
}

// StreamTaskHistory SSE流式推送任务探测历史
// GET /api/tasks/:id/history/stream
// 客户端通过此端点接收指定任务的实时探测结果
func (h *SSEHandler) StreamTaskHistory(c *gin.Context) {
	idStr := c.Param("id")
	taskID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	// 创建SSE客户端，只接收指定任务的探测结果事件
	client := &sse.Client{
		Channel: make(chan sse.Event, 64),
		Filter: func(e sse.Event) bool {
			if e.Type != sse.EventProbeResult {
				return false
			}
			// 检查事件数据中的task_id是否匹配
			return matchTaskID(e.Data, uint(taskID))
		},
	}

	h.serveSSE(c, client)
}

// StreamTaskLogs SSE流式推送任务操作日志
// GET /api/tasks/:id/logs/stream
func (h *SSEHandler) StreamTaskLogs(c *gin.Context) {
	idStr := c.Param("id")
	taskID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	client := &sse.Client{
		Channel: make(chan sse.Event, 64),
		Filter: func(e sse.Event) bool {
			if e.Type != sse.EventOperationLog {
				return false
			}
			return matchTaskID(e.Data, uint(taskID))
		},
	}

	h.serveSSE(c, client)
}

// StreamHealthMonitorResults SSE流式推送健康监控探测结果
// GET /api/health-monitors/:id/results/stream
func (h *SSEHandler) StreamHealthMonitorResults(c *gin.Context) {
	idStr := c.Param("id")
	taskID, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的任务 ID"})
		return
	}

	client := &sse.Client{
		Channel: make(chan sse.Event, 64),
		Filter: func(e sse.Event) bool {
			if e.Type != sse.EventHealthMonitorResult {
				return false
			}
			return matchTaskID(e.Data, uint(taskID))
		},
	}

	h.serveSSE(c, client)
}

// StreamSystemLogs SSE流式推送系统日志（操作日志 + 通知记录）
// GET /api/system-logs/stream
func (h *SSEHandler) StreamSystemLogs(c *gin.Context) {
	client := &sse.Client{
		Channel: make(chan sse.Event, 64),
		Filter: func(e sse.Event) bool {
			// 接收操作日志和通知日志事件
			return e.Type == sse.EventOperationLog || e.Type == sse.EventNotificationLog
		},
	}

	h.serveSSE(c, client)
}

// serveSSE 通用SSE服务函数
// 设置SSE响应头，注册客户端，持续推送事件直到连接断开
func (h *SSEHandler) serveSSE(c *gin.Context, client *sse.Client) {
	hub := sse.GetHub()

	// 设置SSE响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // 禁用Nginx缓冲

	// 注册客户端
	hub.Register(client)
	defer hub.Unregister(client)

	// 获取客户端断开通知
	clientGone := c.Request.Context().Done()

	// 先发送一个连接成功事件
	c.SSEvent("connected", gin.H{"status": "ok"})
	c.Writer.Flush()

	// 持续推送事件
	for {
		select {
		case <-clientGone:
			// 客户端断开连接
			log.Println("[SSE] 客户端断开连接")
			return
		case event, ok := <-client.Channel:
			if !ok {
				// 通道已关闭
				return
			}
			// 序列化事件数据
			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("[SSE] 序列化事件失败: %v", err)
				continue
			}
			// 写入SSE事件
			_, writeErr := io.WriteString(c.Writer, fmt.Sprintf("data: %s\n\n", string(data)))
			if writeErr != nil {
				log.Printf("[SSE] 写入事件失败: %v", writeErr)
				return
			}
			c.Writer.Flush()
		}
	}
}

// matchTaskID 从事件数据中提取task_id并与目标ID比较
// 支持map和struct两种数据格式
func matchTaskID(data interface{}, targetID uint) bool {
	switch v := data.(type) {
	case map[string]interface{}:
		if id, ok := v["task_id"]; ok {
			switch tid := id.(type) {
			case float64:
				return uint(tid) == targetID
			case uint:
				return tid == targetID
			case int:
				return uint(tid) == targetID
			}
		}
	default:
		// 尝试通过JSON序列化再反序列化来提取task_id
		jsonData, err := json.Marshal(v)
		if err != nil {
			return false
		}
		var m map[string]interface{}
		if err := json.Unmarshal(jsonData, &m); err != nil {
			return false
		}
		if id, ok := m["task_id"]; ok {
			if tid, ok := id.(float64); ok {
				return uint(tid) == targetID
			}
		}
	}
	return false
}
