<template>
  <!-- 首次注册页面 -->
  <div class="register-container">
    <el-card class="register-card" shadow="hover">
      <template #header>
        <div class="register-header">
          <h1 class="register-title">DNSHealth 健康检测解析</h1>
          <p class="register-subtitle">首次使用，请创建管理员账户</p>
        </div>
      </template>

      <!-- HTTP 安全风险提示 -->
      <el-alert
        v-if="isHttpAccess"
        title="安全风险提示"
        description="当前通过 HTTP 协议访问，您的注册信息将以明文传输，存在被窃取的风险。建议使用 HTTPS 协议访问本系统。"
        type="warning"
        show-icon
        :closable="false"
        style="margin-bottom: 20px"
      />

      <el-form
        ref="registerFormRef"
        :model="registerForm"
        :rules="registerRules"
        label-width="0"
        size="large"
        @submit.prevent="handleRegister"
      >
        <el-form-item prop="username">
          <el-input
            v-model="registerForm.username"
            placeholder="请设置用户名（至少3位）"
            :prefix-icon="User"
            clearable
          />
        </el-form-item>

        <el-form-item prop="password">
          <el-input
            v-model="registerForm.password"
            type="password"
            placeholder="请设置密码（至少6位）"
            :prefix-icon="Lock"
            show-password
          />
        </el-form-item>

        <el-form-item prop="confirmPassword">
          <el-input
            v-model="registerForm.confirmPassword"
            type="password"
            placeholder="请确认密码"
            :prefix-icon="Lock"
            show-password
            @keyup.enter="handleRegister"
          />
        </el-form-item>

        <el-form-item>
          <el-button
            type="primary"
            :loading="loading"
            class="register-button"
            @click="handleRegister"
          >
            {{ loading ? '注册中...' : '创建账户并登录' }}
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
const registerFormRef = ref(null)
const loading = ref(false)
const isHttpAccess = ref(window.location.protocol === 'http:')

// 页面加载时检查是否还需要注册
onMounted(async () => {
  try {
    const res = await api.get('/setup-status')
    if (!res.data.need_setup) {
      // 已有用户，跳转到登录页
      router.replace('/login')
    }
  } catch {
    // 忽略
  }
})

const registerForm = reactive({
  username: '',
  password: '',
  confirmPassword: ''
})

// 确认密码校验器
const validateConfirmPassword = (rule, value, callback) => {
  if (value !== registerForm.password) {
    callback(new Error('两次输入的密码不一致'))
  } else {
    callback()
  }
}

const registerRules = reactive({
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    { min: 3, max: 32, message: '用户名长度为 3-32 位', trigger: 'blur' }
  ],
  password: [
    { required: true, message: '请输入密码', trigger: 'blur' },
    { min: 6, max: 64, message: '密码长度为 6-64 位', trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: '请确认密码', trigger: 'blur' },
    { validator: validateConfirmPassword, trigger: 'blur' }
  ]
})

const handleRegister = async () => {
  if (!registerFormRef.value) return
  const valid = await registerFormRef.value.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    const res = await api.post('/register', {
      username: registerForm.username,
      password: registerForm.password,
      confirm_password: registerForm.confirmPassword
    })
    localStorage.setItem('token', res.data.token)
    ElMessage.success('注册成功，已自动登录')
    router.push('/')
  } catch (error) {
    const msg = error.response?.data?.error || '注册失败'
    ElMessage.error(msg)
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.register-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}
.register-card {
  width: 420px;
  border-radius: 12px;
}
.register-header {
  text-align: center;
}
.register-title {
  margin: 0;
  font-size: 24px;
  color: #303133;
  font-weight: 600;
}
.register-subtitle {
  margin: 8px 0 0;
  font-size: 14px;
  color: #909399;
}
.register-button {
  width: 100%;
}
</style>
