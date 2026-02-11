<template>
  <!-- 登录页面 -->
  <div class="login-container">
    <el-card class="login-card" shadow="hover">
      <!-- 标题区域 -->
      <template #header>
        <div class="login-header">
          <h1 class="login-title">DNSHealth 健康检测解析</h1>
          <p class="login-subtitle">请登录以继续</p>
        </div>
      </template>

      <!-- HTTP 安全风险提示 -->
      <el-alert
        v-if="isHttpAccess"
        title="安全风险提示"
        description="当前通过 HTTP 协议访问，您的登录凭据将以明文传输，存在被窃取的风险。建议使用 HTTPS 协议访问本系统。"
        type="warning"
        show-icon
        :closable="false"
        style="margin-bottom: 20px"
      />

      <!-- 登录表单 -->
      <el-form
        ref="loginFormRef"
        :model="loginForm"
        :rules="loginRules"
        label-width="0"
        size="large"
        @submit.prevent="handleLogin"
      >
        <!-- 用户名输入框 -->
        <el-form-item prop="username">
          <el-input
            v-model="loginForm.username"
            placeholder="请输入用户名"
            :prefix-icon="User"
            clearable
          />
        </el-form-item>

        <!-- 密码输入框 -->
        <el-form-item prop="password">
          <el-input
            v-model="loginForm.password"
            type="password"
            placeholder="请输入密码"
            :prefix-icon="Lock"
            show-password
            @keyup.enter="handleLogin"
          />
        </el-form-item>

        <!-- 登录按钮 -->
        <el-form-item>
          <el-button
            type="primary"
            :loading="loading"
            class="login-button"
            @click="handleLogin"
          >
            {{ loading ? '登录中...' : '登 录' }}
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { User, Lock } from '@element-plus/icons-vue'
import api from '../api'

const router = useRouter()
const loginFormRef = ref(null)
const loading = ref(false)

// 检测是否通过 HTTP 访问
const isHttpAccess = ref(window.location.protocol === 'http:')

const loginForm = reactive({
  username: '',
  password: ''
})

const loginRules = reactive({
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' }
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' }
  ]
})

// 页面加载时检查是否需要初始化注册
onMounted(async () => {
  try {
    const res = await api.get('/setup-status')
    if (res.data.need_setup) {
      router.replace('/register')
    }
  } catch {
    // 查询失败不影响正常登录流程
  }
})

const handleLogin = async () => {
  if (!loginFormRef.value) return
  const valid = await loginFormRef.value.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    const response = await api.post('/login', {
      username: loginForm.username,
      password: loginForm.password
    })
    const { token } = response.data
    localStorage.setItem('token', token)
    ElMessage.success('登录成功')
    router.push('/')
  } catch {
    ElMessage.error('用户名或密码错误')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}
.login-card {
  width: 420px;
  border-radius: 12px;
}
.login-header {
  text-align: center;
}
.login-title {
  margin: 0;
  font-size: 24px;
  color: #303133;
  font-weight: 600;
}
.login-subtitle {
  margin: 8px 0 0;
  font-size: 14px;
  color: #909399;
}
.login-button {
  width: 100%;
}
</style>
