<template>
  <div class="account-container">
    <h2>账户管理</h2>

    <!-- 修改用户名 -->
    <el-card shadow="never" style="margin-bottom: 20px">
      <template #header>
        <span>修改用户名</span>
      </template>
      <el-form
        ref="usernameFormRef"
        :model="usernameForm"
        :rules="usernameRules"
        label-width="100px"
        @submit.prevent="handleChangeUsername"
      >
        <el-form-item label="当前用户名">
          <el-text>{{ currentUsername }}</el-text>
        </el-form-item>
        <el-form-item label="新用户名" prop="newUsername">
          <el-input v-model="usernameForm.newUsername" placeholder="请输入新用户名（至少3位）" />
        </el-form-item>
        <el-form-item label="确认密码" prop="password">
          <el-input v-model="usernameForm.password" type="password" placeholder="请输入当前密码以确认" show-password />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="usernameLoading" @click="handleChangeUsername">保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 修改密码 -->
    <el-card shadow="never">
      <template #header>
        <span>修改密码</span>
      </template>
      <el-form
        ref="passwordFormRef"
        :model="passwordForm"
        :rules="passwordRules"
        label-width="100px"
        @submit.prevent="handleChangePassword"
      >
        <el-form-item label="原密码" prop="oldPassword">
          <el-input v-model="passwordForm.oldPassword" type="password" placeholder="请输入原密码" show-password />
        </el-form-item>
        <el-form-item label="新密码" prop="newPassword">
          <el-input v-model="passwordForm.newPassword" type="password" placeholder="请输入新密码（至少6位）" show-password />
        </el-form-item>
        <el-form-item label="确认新密码" prop="confirmPassword">
          <el-input v-model="passwordForm.confirmPassword" type="password" placeholder="请再次输入新密码" show-password />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="passwordLoading" @click="handleChangePassword">保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

const currentUsername = ref('')
const usernameFormRef = ref(null)
const passwordFormRef = ref(null)
const usernameLoading = ref(false)
const passwordLoading = ref(false)

const usernameForm = reactive({
  newUsername: '',
  password: ''
})

const passwordForm = reactive({
  oldPassword: '',
  newPassword: '',
  confirmPassword: ''
})

const usernameRules = reactive({
  newUsername: [
    { required: true, message: '请输入新用户名', trigger: 'blur' },
    { min: 3, max: 32, message: '用户名长度为 3-32 位', trigger: 'blur' }
  ],
  password: [
    { required: true, message: '请输入当前密码', trigger: 'blur' }
  ]
})

const validateConfirmPassword = (rule, value, callback) => {
  if (value !== passwordForm.newPassword) {
    callback(new Error('两次输入的新密码不一致'))
  } else {
    callback()
  }
}

const passwordRules = reactive({
  oldPassword: [
    { required: true, message: '请输入原密码', trigger: 'blur' }
  ],
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { min: 6, max: 64, message: '密码长度为 6-64 位', trigger: 'blur' }
  ],
  confirmPassword: [
    { required: true, message: '请确认新密码', trigger: 'blur' },
    { validator: validateConfirmPassword, trigger: 'blur' }
  ]
})

onMounted(async () => {
  try {
    const res = await api.get('/account')
    currentUsername.value = res.data.username
  } catch {
    // 忽略
  }
})

const handleChangeUsername = async () => {
  if (!usernameFormRef.value) return
  const valid = await usernameFormRef.value.validate().catch(() => false)
  if (!valid) return

  usernameLoading.value = true
  try {
    const res = await api.put('/account/username', {
      new_username: usernameForm.newUsername,
      password: usernameForm.password
    })
    // 更新 token 和显示的用户名
    localStorage.setItem('token', res.data.token)
    currentUsername.value = res.data.username
    usernameForm.newUsername = ''
    usernameForm.password = ''
    ElMessage.success('用户名修改成功')
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '修改用户名失败')
  } finally {
    usernameLoading.value = false
  }
}

const handleChangePassword = async () => {
  if (!passwordFormRef.value) return
  const valid = await passwordFormRef.value.validate().catch(() => false)
  if (!valid) return

  passwordLoading.value = true
  try {
    await api.put('/account/password', {
      old_password: passwordForm.oldPassword,
      new_password: passwordForm.newPassword,
      confirm_password: passwordForm.confirmPassword
    })
    passwordForm.oldPassword = ''
    passwordForm.newPassword = ''
    passwordForm.confirmPassword = ''
    ElMessage.success('密码修改成功')
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '修改密码失败')
  } finally {
    passwordLoading.value = false
  }
}
</script>

<style scoped>
.account-container {
  max-width: 600px;
}
.account-container h2 {
  margin-bottom: 20px;
  color: #303133;
}
</style>
