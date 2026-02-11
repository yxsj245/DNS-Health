import axios from 'axios'
import router from './router'

// 创建 axios 实例，统一配置基础 URL
const api = axios.create({
  baseURL: '/api',
  timeout: 10000
})

// 请求拦截器：自动附加 JWT Token
api.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('token')
    if (token) {
      config.headers.Authorization = `Bearer ${token}`
    }
    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// 响应拦截器：处理 401 未授权，自动跳转登录页
api.interceptors.response.use(
  (response) => {
    return response
  },
  (error) => {
    if (error.response && error.response.status === 401) {
      // 清除过期的 token
      localStorage.removeItem('token')
      // 重定向到登录页
      router.push('/login')
    }
    return Promise.reject(error)
  }
)

export default api
