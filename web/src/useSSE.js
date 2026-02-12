/**
 * SSE（Server-Sent Events）连接管理工具
 * 提供可复用的SSE连接逻辑，支持自动重连、JWT认证
 */
import { ref, onBeforeUnmount } from 'vue'

/**
 * 创建SSE连接的组合式函数
 * @param {Object} options - 配置选项
 * @param {Function} options.onMessage - 收到消息时的回调函数，参数为解析后的事件对象
 * @param {Function} [options.onConnected] - 连接成功时的回调
 * @param {Function} [options.onError] - 连接错误时的回调
 * @param {number} [options.reconnectDelay=3000] - 重连延迟（毫秒）
 * @param {number} [options.maxRetries=10] - 最大重试次数
 * @returns {Object} { connected, connect, disconnect }
 */
export function useSSE(options = {}) {
  const {
    onMessage,
    onConnected,
    onError,
    reconnectDelay = 3000,
    maxRetries = 10
  } = options

  const connected = ref(false)
  let eventSource = null
  let retryCount = 0
  let reconnectTimer = null
  let currentUrl = ''

  /**
   * 建立SSE连接
   * @param {string} url - SSE端点路径
   */
  const connect = (url) => {
    // 先断开旧连接
    disconnect()
    currentUrl = url

    const token = localStorage.getItem('token')
    if (!token) {
      console.warn('[SSE] 未找到JWT token，跳过连接')
      return
    }

    // EventSource不支持自定义Header，通过URL参数传递token
    const separator = url.includes('?') ? '&' : '?'
    const fullUrl = `${url}${separator}token=${encodeURIComponent(token)}`

    try {
      eventSource = new EventSource(fullUrl)

      // 连接成功事件
      eventSource.addEventListener('connected', () => {
        connected.value = true
        retryCount = 0
        onConnected?.()
      })

      // 数据消息事件
      eventSource.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data)
          onMessage?.(data)
        } catch (err) {
          console.error('[SSE] 解析消息失败:', err)
        }
      }

      // 错误处理和自动重连
      eventSource.onerror = () => {
        connected.value = false
        eventSource?.close()
        eventSource = null
        onError?.()

        // 自动重连
        if (retryCount < maxRetries && currentUrl) {
          retryCount++
          reconnectTimer = setTimeout(() => connect(currentUrl), reconnectDelay)
        }
      }
    } catch (err) {
      console.error('[SSE] 创建连接失败:', err)
    }
  }

  /**
   * 断开SSE连接
   */
  const disconnect = () => {
    if (reconnectTimer) {
      clearTimeout(reconnectTimer)
      reconnectTimer = null
    }
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    connected.value = false
    retryCount = 0
    currentUrl = ''
  }

  // 组件卸载时自动断开连接
  onBeforeUnmount(() => {
    disconnect()
  })

  return {
    connected,
    connect,
    disconnect
  }
}
