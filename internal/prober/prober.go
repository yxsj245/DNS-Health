// Package prober 健康探测模块，支持多协议健康检测
package prober

import (
	"context"
	"time"
)

// ProbeProtocol 探测协议类型
type ProbeProtocol string

const (
	// ProbeICMP ICMP 探测协议
	ProbeICMP ProbeProtocol = "ICMP"
	// ProbeTCP TCP 探测协议
	ProbeTCP ProbeProtocol = "TCP"
	// ProbeUDP UDP 探测协议
	ProbeUDP ProbeProtocol = "UDP"
	// ProbeHTTP HTTP 探测协议
	ProbeHTTP ProbeProtocol = "HTTP"
	// ProbeHTTPS HTTPS 探测协议
	ProbeHTTPS ProbeProtocol = "HTTPS"
)

// ValidProtocols 所有有效的探测协议列表
var ValidProtocols = []ProbeProtocol{
	ProbeICMP,
	ProbeTCP,
	ProbeUDP,
	ProbeHTTP,
	ProbeHTTPS,
}

// IsValidProtocol 检查给定的协议是否为有效的探测协议
func IsValidProtocol(protocol ProbeProtocol) bool {
	for _, p := range ValidProtocols {
		if p == protocol {
			return true
		}
	}
	return false
}

// ProbeResult 探测结果
type ProbeResult struct {
	// Success 探测是否成功
	Success bool
	// Latency 响应延迟
	Latency time.Duration
	// Time 探测时间戳
	Time time.Time
	// Error 错误信息（探测失败时记录）
	Error string
}

// Prober 探测器接口
// 所有协议的探测器都需要实现此接口
type Prober interface {
	// Probe 执行一次健康探测
	// ctx: 上下文，用于取消控制
	// target: 目标 IP 地址
	// port: 目标端口（ICMP 协议忽略此参数）
	// timeout: 超时时间
	// 返回探测结果
	Probe(ctx context.Context, target string, port int, timeout time.Duration) ProbeResult
}

// NewProber 根据协议创建对应的探测器实例
// 对于未知协议返回 nil
func NewProber(protocol ProbeProtocol) Prober {
	switch protocol {
	case ProbeICMP:
		return &ICMPProber{}
	case ProbeTCP:
		return &TCPProber{}
	case ProbeUDP:
		return &UDPProber{}
	case ProbeHTTP:
		return &HTTPProber{}
	case ProbeHTTPS:
		return &HTTPSProber{}
	default:
		return nil
	}
}
