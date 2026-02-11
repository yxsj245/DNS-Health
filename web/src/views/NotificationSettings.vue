<template>
  <!-- 通知设置页面：SMTP 配置 + 任务通知偏好 -->
  <div class="notification-settings-page">
    <!-- 页面头部 -->
    <div class="page-header">
      <h2 class="page-title">通知设置</h2>
    </div>

    <!-- SMTP 配置区域 -->
    <el-card class="settings-card" shadow="never">
      <template #header>
        <div class="card-header">
          <span class="card-title">SMTP 邮件服务器配置</span>
        </div>
      </template>

      <el-form
        ref="smtpFormRef"
        :model="smtpForm"
        :rules="smtpRules"
        label-width="120px"
        v-loading="smtpLoading"
      >
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="服务器地址" prop="host">
              <el-input v-model="smtpForm.host" placeholder="例如 smtp.example.com" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="端口" prop="port">
              <el-input-number v-model="smtpForm.port" :min="1" :max="65535" style="width: 100%" />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="用户名" prop="username">
              <el-input v-model="smtpForm.username" placeholder="SMTP 登录用户名" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="密码" prop="password">
              <el-input
                v-model="smtpForm.password"
                type="password"
                show-password
                placeholder="SMTP 登录密码"
              />
            </el-form-item>
          </el-col>
        </el-row>
        <el-row :gutter="20">
          <el-col :span="12">
            <el-form-item label="发件人地址" prop="from_address">
              <el-input v-model="smtpForm.from_address" placeholder="例如 noreply@example.com" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="收件人地址" prop="to_address">
              <el-input v-model="smtpForm.to_address" placeholder="例如 admin@example.com" />
            </el-form-item>
          </el-col>
        </el-row>

        <!-- 操作按钮 -->
        <el-form-item>
          <el-button type="primary" :loading="saveLoading" @click="handleSaveSMTP">
            保存配置
          </el-button>
          <el-button :loading="testLoading" @click="handleTestSMTP">
            测试连接
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 任务通知设置区域 -->
    <el-card class="settings-card" shadow="never">
      <template #header>
        <div class="card-header">
          <span class="card-title">任务通知设置</span>
          <div class="card-actions">
            <el-button type="primary" size="small" :loading="batchLoading" @click="handleBatchEnable">
              全部启用
            </el-button>
            <el-button size="small" :loading="batchLoading" @click="handleBatchDisable">
              全部禁用
            </el-button>
          </div>
        </div>
      </template>

      <el-table
        :data="notificationSettings"
        v-loading="settingsLoading"
        border
        stripe
        style="width: 100%"
        empty-text="暂无探测任务"
      >
        <el-table-column prop="task_name" label="任务名称" min-width="200" />
        <el-table-column label="故障转移" min-width="120" align="center">
          <template #default="{ row }">
            <el-switch
              v-model="row.notify_failover"
              @change="handleSettingChange(row)"
            />
          </template>
        </el-table-column>
        <el-table-column label="恢复" min-width="120" align="center">
          <template #default="{ row }">
            <el-switch
              v-model="row.notify_recovery"
              @change="handleSettingChange(row)"
            />
          </template>
        </el-table-column>
        <el-table-column label="连续失败" min-width="120" align="center">
          <template #default="{ row }">
            <el-switch
              v-model="row.notify_consec_fail"
              @change="handleSettingChange(row)"
            />
          </template>
        </el-table-column>
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

// ==================== 状态定义 ====================

// SMTP 表单引用
const smtpFormRef = ref(null)

// SMTP 配置加载状态
const smtpLoading = ref(false)

// 保存按钮加载状态
const saveLoading = ref(false)

// 测试连接按钮加载状态
const testLoading = ref(false)

// 任务通知设置加载状态
const settingsLoading = ref(false)

// 批量操作加载状态
const batchLoading = ref(false)

// SMTP 配置表单数据
const smtpForm = reactive({
  host: '',
  port: 465,
  username: '',
  password: '',
  from_address: '',
  to_address: ''
})

// 任务通知设置列表
const notificationSettings = ref([])

// ==================== 表单验证规则 ====================

const smtpRules = reactive({
  host: [{ required: true, message: '请输入 SMTP 服务器地址', trigger: 'blur' }],
  port: [{ required: true, message: '请输入端口号', trigger: 'blur' }],
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }],
  from_address: [{ required: true, message: '请输入发件人地址', trigger: 'blur' }],
  to_address: [{ required: true, message: '请输入收件人地址', trigger: 'blur' }]
})

// ==================== API 调用 ====================

/**
 * 获取 SMTP 配置
 * 调用 GET /api/notification/smtp-config
 */
const fetchSMTPConfig = async () => {
  smtpLoading.value = true
  try {
    const response = await api.get('/notification/smtp-config')
    const data = response.data.data
    if (data) {
      smtpForm.host = data.host || ''
      smtpForm.port = data.port || 465
      smtpForm.username = data.username || ''
      smtpForm.password = data.password || ''
      smtpForm.from_address = data.from_address || ''
      smtpForm.to_address = data.to_address || ''
    }
  } catch (error) {
    ElMessage.error('获取 SMTP 配置失败')
  } finally {
    smtpLoading.value = false
  }
}

/**
 * 保存 SMTP 配置
 * 调用 PUT /api/notification/smtp-config
 */
const handleSaveSMTP = async () => {
  if (!smtpFormRef.value) return
  const valid = await smtpFormRef.value.validate().catch(() => false)
  if (!valid) return

  saveLoading.value = true
  try {
    await api.put('/notification/smtp-config', {
      host: smtpForm.host,
      port: smtpForm.port,
      username: smtpForm.username,
      password: smtpForm.password,
      from_address: smtpForm.from_address,
      to_address: smtpForm.to_address
    })
    ElMessage.success('SMTP 配置保存成功')
  } catch (error) {
    const msg = error.response?.data?.error || '保存 SMTP 配置失败'
    ElMessage.error(msg)
  } finally {
    saveLoading.value = false
  }
}

/**
 * 测试 SMTP 连接
 * 调用 POST /api/notification/smtp-test
 */
const handleTestSMTP = async () => {
  if (!smtpFormRef.value) return
  const valid = await smtpFormRef.value.validate().catch(() => false)
  if (!valid) return

  testLoading.value = true
  try {
    await api.post('/notification/smtp-test', {
      host: smtpForm.host,
      port: smtpForm.port,
      username: smtpForm.username,
      password: smtpForm.password,
      from_address: smtpForm.from_address,
      to_address: smtpForm.to_address
    })
    ElMessage.success('SMTP 连接测试成功，测试邮件已发送')
  } catch (error) {
    const msg = error.response?.data?.error || 'SMTP 连接测试失败'
    ElMessage.error(msg)
  } finally {
    testLoading.value = false
  }
}

/**
 * 获取所有任务的通知设置
 * 调用 GET /api/notification/settings
 */
const fetchNotificationSettings = async () => {
  settingsLoading.value = true
  try {
    const response = await api.get('/notification/settings')
    notificationSettings.value = response.data || []
  } catch (error) {
    ElMessage.error('获取通知设置失败')
  } finally {
    settingsLoading.value = false
  }
}

/**
 * 更新单个任务的通知设置
 * 调用 PUT /api/notification/settings/:taskId
 * @param {Object} row - 任务通知设置行数据
 */
const handleSettingChange = async (row) => {
  try {
    await api.put(`/notification/settings/${row.task_id}`, {
      notify_failover: row.notify_failover,
      notify_recovery: row.notify_recovery,
      notify_consec_fail: row.notify_consec_fail
    })
    ElMessage.success('通知设置已保存')
  } catch (error) {
    ElMessage.error('更新通知设置失败')
    // 更新失败时重新加载设置，恢复原始状态
    await fetchNotificationSettings()
  }
}

/**
 * 批量启用所有任务的通知
 * 调用 PUT /api/notification/settings/batch
 */
const handleBatchEnable = async () => {
  batchLoading.value = true
  try {
    await api.put('/notification/settings/batch', { enable_all: true })
    ElMessage.success('已批量启用所有任务的通知')
    await fetchNotificationSettings()
  } catch (error) {
    ElMessage.error('批量启用通知失败')
  } finally {
    batchLoading.value = false
  }
}

/**
 * 批量禁用所有任务的通知
 * 调用 PUT /api/notification/settings/batch
 */
const handleBatchDisable = async () => {
  batchLoading.value = true
  try {
    await api.put('/notification/settings/batch', { enable_all: false })
    ElMessage.success('已批量禁用所有任务的通知')
    await fetchNotificationSettings()
  } catch (error) {
    ElMessage.error('批量禁用通知失败')
  } finally {
    batchLoading.value = false
  }
}

// ==================== 生命周期 ====================

// 页面加载时获取 SMTP 配置和通知设置
onMounted(() => {
  fetchSMTPConfig()
  fetchNotificationSettings()
})
</script>

<style scoped>
/* 通知设置页面容器 */
.notification-settings-page {
  padding: 20px;
}

/* 页面头部 */
.page-header {
  margin-bottom: 20px;
}

/* 页面标题样式 */
.page-title {
  margin: 0;
  font-size: 20px;
  color: #303133;
  font-weight: 600;
}

/* 设置卡片 */
.settings-card {
  margin-bottom: 20px;
}

/* 卡片头部 */
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

/* 卡片标题 */
.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

/* 卡片操作按钮区域 */
.card-actions {
  display: flex;
  gap: 8px;
}
</style>
