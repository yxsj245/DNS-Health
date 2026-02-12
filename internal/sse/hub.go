// Package sse 实现 Server-Sent Events 事件总线
// 用于将探测结果、操作日志等实时推送到前端
package sse

import (
	"encoding/json"
	"log"
	"sync"
)

// EventType 事件类型
type EventType string

const (
	// EventProbeResult 探测结果事件（任务探测历史）
	EventProbeResult EventType = "probe_result"
	// EventOperationLog 操作日志事件
	EventOperationLog EventType = "operation_log"
	// EventNotificationLog 通知日志事件
	EventNotificationLog EventType = "notification_log"
	// EventHealthMonitorResult 健康监控探测结果事件
	EventHealthMonitorResult EventType = "health_monitor_result"
)

// Event SSE事件
type Event struct {
	Type EventType   `json:"type"`
	Data interface{} `json:"data"`
}

// Client SSE客户端连接
type Client struct {
	// Channel 事件通道，用于向客户端推送事件
	Channel chan Event
	// Filter 过滤函数，返回true表示该客户端需要接收此事件
	Filter func(Event) bool
}

// Hub SSE事件总线，管理所有客户端连接和事件分发
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

// 全局事件总线实例
var globalHub *Hub
var once sync.Once

// GetHub 获取全局SSE事件总线实例（单例）
func GetHub() *Hub {
	once.Do(func() {
		globalHub = &Hub{
			clients: make(map[*Client]struct{}),
		}
	})
	return globalHub
}

// Register 注册一个新的SSE客户端
func (h *Hub) Register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[client] = struct{}{}
}

// Unregister 注销一个SSE客户端，关闭其通道
func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[client]; ok {
		close(client.Channel)
		delete(h.clients, client)
	}
}

// Publish 发布事件到所有匹配的客户端
// 非阻塞发送，如果客户端通道已满则跳过（避免慢客户端阻塞整个系统）
func (h *Hub) Publish(event Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		// 如果客户端设置了过滤器，检查是否需要接收此事件
		if client.Filter != nil && !client.Filter(event) {
			continue
		}
		// 非阻塞发送
		select {
		case client.Channel <- event:
		default:
			// 通道已满，跳过此客户端，避免阻塞
			log.Printf("[SSE] 客户端通道已满，跳过事件推送: %s", event.Type)
		}
	}
}

// PublishJSON 发布事件，data会被序列化为JSON
func (h *Hub) PublishJSON(eventType EventType, data interface{}) {
	h.Publish(Event{
		Type: eventType,
		Data: data,
	})
}

// MarshalEvent 将事件序列化为SSE格式的字节
func MarshalEvent(event Event) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	// SSE格式: "data: {json}\n\n"
	result := append([]byte("data: "), data...)
	result = append(result, '\n', '\n')
	return result, nil
}
