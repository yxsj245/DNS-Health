// Package retry 提供通用的重试和指数退避策略工具。
// 用于包装可能失败的函数调用（如DNS Provider API），在遇到临时错误或限流时自动重试。
// 支持可配置的最大重试次数、初始延迟、最大延迟和延迟倍增因子。
//
// 核心功能：
// - 通用重试函数 Do：包装任意函数调用，自动处理重试逻辑
// - 指数退避延迟计算 CalculateBackoffDelay：根据重试次数计算延迟时间
// - 限流错误检测 IsRateLimitError：识别HTTP 429和Throttling类错误
//
// 验证需求：12.5（API失败重试）、12.6（限流退避策略）
package retry

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

// Config 重试配置
// 定义重试行为的所有可配置参数
type Config struct {
	// MaxRetries 最大重试次数（不包括首次调用）
	MaxRetries int
	// InitialDelay 首次重试的初始延迟时间
	InitialDelay time.Duration
	// MaxDelay 延迟时间的上限，防止延迟过长
	MaxDelay time.Duration
	// Multiplier 每次重试延迟的倍增因子
	Multiplier float64
}

// DefaultConfig 返回默认的重试配置
// 默认值：最多重试3次，初始延迟1秒，最大延迟30秒，倍增因子2.0
func DefaultConfig() Config {
	return Config{
		MaxRetries:   3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
	}
}

// CalculateBackoffDelay 计算指数退避延迟时间（导出函数，便于属性测试）
// attempt: 当前重试次数（从0开始，0表示第一次重试）
// config: 重试配置
// 返回值: 计算出的延迟时间，不超过 MaxDelay
//
// 计算公式: delay = min(InitialDelay * Multiplier^attempt, MaxDelay)
//
// 示例（默认配置）：
//   - attempt=0: 1s * 2^0 = 1s
//   - attempt=1: 1s * 2^1 = 2s
//   - attempt=2: 1s * 2^2 = 4s
//   - attempt=3: 1s * 2^3 = 8s
func CalculateBackoffDelay(attempt int, config Config) time.Duration {
	if attempt < 0 {
		attempt = 0
	}

	// 计算延迟: InitialDelay * Multiplier^attempt
	delayFloat := float64(config.InitialDelay) * math.Pow(config.Multiplier, float64(attempt))

	// 防止溢出：如果计算结果超过float64可表示的范围，直接返回MaxDelay
	if math.IsInf(delayFloat, 0) || math.IsNaN(delayFloat) || delayFloat < 0 {
		return config.MaxDelay
	}

	delay := time.Duration(delayFloat)

	// 确保不超过最大延迟
	if delay > config.MaxDelay {
		return config.MaxDelay
	}

	// 确保延迟不为负数（防御性编程）
	if delay < 0 {
		return config.MaxDelay
	}

	return delay
}

// IsRateLimitError 判断错误是否为限流错误（导出函数，便于属性测试）
// 检测以下情况：
// - 错误信息包含 "Throttling"（阿里云限流错误码）
// - 错误信息包含 "429"（HTTP 429 Too Many Requests）
// - 错误信息包含 "rate limit"（通用限流描述）
// - 错误信息包含 "too many requests"（通用限流描述）
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := strings.ToLower(err.Error())

	// 检测阿里云限流错误码
	if strings.Contains(errMsg, "throttling") {
		return true
	}

	// 检测HTTP 429状态码
	if strings.Contains(errMsg, "429") {
		return true
	}

	// 检测通用限流描述
	if strings.Contains(errMsg, "rate limit") {
		return true
	}

	// 检测通用限流描述
	if strings.Contains(errMsg, "too many requests") {
		return true
	}

	return false
}

// Do 执行带重试的函数调用
// 对于限流错误使用指数退避策略，对于其他错误直接重试
// ctx: 上下文，用于控制超时和取消
// config: 重试配置
// fn: 要执行的函数，返回error表示是否成功
// 返回值: 最终的错误（如果所有重试都失败）
//
// 行为说明：
// 1. 首先执行fn，如果成功则直接返回nil
// 2. 如果失败且为限流错误，使用指数退避延迟后重试
// 3. 如果失败且为非限流错误，使用初始延迟后重试
// 4. 重试次数达到MaxRetries后，返回最后一次的错误
// 5. 在等待延迟期间，如果context被取消，立即返回context错误
func Do(ctx context.Context, config Config, fn func() error) error {
	var lastErr error

	// 首次调用 + MaxRetries次重试
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// 执行目标函数
		lastErr = fn()
		if lastErr == nil {
			return nil // 成功，直接返回
		}

		// 如果已经是最后一次尝试，不再等待
		if attempt == config.MaxRetries {
			break
		}

		// 计算延迟时间
		var delay time.Duration
		if IsRateLimitError(lastErr) {
			// 限流错误：使用指数退避策略
			delay = CalculateBackoffDelay(attempt, config)
		} else {
			// 非限流错误：使用固定的初始延迟
			delay = config.InitialDelay
		}

		// 等待延迟或context取消
		select {
		case <-ctx.Done():
			return fmt.Errorf("重试被取消: %w（最后一次错误: %v）", ctx.Err(), lastErr)
		case <-time.After(delay):
			// 继续重试
		}
	}

	return fmt.Errorf("重试 %d 次后仍然失败: %w", config.MaxRetries, lastErr)
}

// DoWithResult 执行带重试的函数调用（支持返回值）
// 与Do类似，但支持函数返回一个结果值
// ctx: 上下文
// config: 重试配置
// fn: 要执行的函数，返回结果和错误
// 返回值: 函数的结果和最终的错误
func DoWithResult[T any](ctx context.Context, config Config, fn func() (T, error)) (T, error) {
	var lastResult T
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		lastResult, lastErr = fn()
		if lastErr == nil {
			return lastResult, nil
		}

		if attempt == config.MaxRetries {
			break
		}

		var delay time.Duration
		if IsRateLimitError(lastErr) {
			delay = CalculateBackoffDelay(attempt, config)
		} else {
			delay = config.InitialDelay
		}

		select {
		case <-ctx.Done():
			var zero T
			return zero, fmt.Errorf("重试被取消: %w（最后一次错误: %v）", ctx.Err(), lastErr)
		case <-time.After(delay):
		}
	}

	return lastResult, fmt.Errorf("重试 %d 次后仍然失败: %w", config.MaxRetries, lastErr)
}
