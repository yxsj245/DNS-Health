<template>
  <!-- 健康监控任务表单页面：创建/编辑模式 -->
  <div class="task-form-page">
    <!-- 页面头部 -->
    <div class="page-header">
      <h2 class="page-title">{{ isEditMode ? '编辑健康监控任务' : '创建健康监控任务' }}</h2>
    </div>

    <!-- 表单内容：分组卡片布局 -->
    <div class="wizard-content wizard-form-content" v-loading="pageLoading">
      <el-form
        ref="formRef"
        :model="form"
        :rules="formRules"
        label-width="120px"
      >
        <!-- 域名配置卡片 -->
        <el-card class="form-section" shadow="never">
          <template #header>
            <div class="section-header">
              <el-icon><Position /></el-icon>
              <span>域名配置</span>
            </div>
          </template>
          <el-form-item label="凭证" prop="credential_id">
            <el-select v-model="form.credential_id" placeholder="请选择云服务商凭证（可选）" clearable style="width: 100%">
              <el-option v-for="cred in credentials" :key="cred.id" :label="cred.name" :value="cred.id" />
            </el-select>
            <div v-if="!form.credential_id" class="credential-hint">
              <el-text type="warning" size="small">
                未选择凭证时，系统将直接通过DNS解析域名获取IP地址进行探测，每次探测周期都会重新解析。
              </el-text>
            </div>
          </el-form-item>
          <el-form-item label="域名" prop="domain">
            <el-input v-model="form.domain" placeholder="请输入域名，例如 example.com" />
          </el-form-item>
          <el-form-item label="主机记录" prop="sub_domain">
            <el-input v-model="form.sub_domain" placeholder="@ 表示根域名" />
          </el-form-item>
          <el-form-item label="记录类型" prop="record_type">
            <el-select v-model="form.record_type" placeholder="请选择记录类型" style="width: 100%">
              <el-option label="A" value="A" />
              <el-option label="AAAA" value="AAAA" />
              <el-option label="A_AAAA" value="A_AAAA" />
              <el-option label="CNAME" value="CNAME" />
            </el-select>
          </el-form-item>
        </el-card>

        <!-- 探测配置卡片 -->
        <el-card class="form-section" shadow="never">
          <template #header>
            <div class="section-header">
              <el-icon><Monitor /></el-icon>
              <span>探测配置</span>
            </div>
          </template>
          <!-- 响应式布局：小屏幕堆叠，大屏幕并排 -->
          <el-row :gutter="20">
            <el-col :xs="24" :sm="12">
              <el-form-item label="探测协议" prop="probe_protocol">
                <el-select v-model="form.probe_protocol" placeholder="请选择探测协议" style="width: 100%" @change="onProtocolChange">
                  <el-option label="ICMP" value="ICMP" />
                  <el-option label="TCP" value="TCP" />
                  <el-option label="UDP" value="UDP" />
                  <el-option label="HTTP" value="HTTP" />
                  <el-option label="HTTPS" value="HTTPS" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col v-if="showPortField" :xs="24" :sm="12">
              <el-form-item label="探测端口" prop="probe_port">
                <el-input-number v-model="form.probe_port" :min="1" :max="65535" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="20">
            <el-col :xs="24" :sm="12">
              <el-form-item label="探测周期" prop="probe_interval_sec">
                <el-input-number v-model="form.probe_interval_sec" :min="1" style="width: 100%" />
                <span class="form-unit">秒</span>
              </el-form-item>
            </el-col>
            <el-col :xs="24" :sm="12">
              <el-form-item label="超时时间" prop="timeout_ms">
                <el-input-number v-model="form.timeout_ms" :min="100" style="width: 100%" />
                <span class="form-unit">毫秒</span>
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="20">
            <el-col :xs="24" :sm="12">
              <el-form-item label="失败阈值" prop="fail_threshold">
                <el-tooltip content="连续探测失败达到此次数后，将标记为不健康" placement="top">
                  <el-input-number v-model="form.fail_threshold" :min="1" style="width: 100%" />
                </el-tooltip>
              </el-form-item>
            </el-col>
            <el-col :xs="24" :sm="12">
              <el-form-item label="恢复阈值" prop="recover_threshold">
                <el-tooltip content="连续探测成功达到此次数后，将恢复为健康状态" placement="top">
                  <el-input-number v-model="form.recover_threshold" :min="1" style="width: 100%" />
                </el-tooltip>
              </el-form-item>
            </el-col>
          </el-row>
        </el-card>

        <!-- CNAME专用配置卡片（仅记录类型为CNAME时显示） -->
        <el-card v-if="showCnameConfig" class="form-section" shadow="never">
          <template #header>
            <div class="section-header">
              <el-icon><Link /></el-icon>
              <span>CNAME 阈值配置</span>
            </div>
          </template>
          <el-form-item label="阈值类型" prop="fail_threshold_type">
            <el-select v-model="form.fail_threshold_type" placeholder="请选择阈值类型" style="width: 100%">
              <el-option label="个数" value="count" />
              <el-option label="百分比" value="percent" />
            </el-select>
          </el-form-item>
          <el-form-item label="阈值数值" prop="fail_threshold_value">
            <!-- 百分比类型使用滑动条 -->
            <el-slider
              v-if="form.fail_threshold_type === 'percent'"
              v-model="form.fail_threshold_value"
              :min="1"
              :max="100"
              :show-tooltip="true"
              :format-tooltip="(val) => val + '%'"
              style="width: 100%"
            />
            <!-- 个数类型使用数字输入框 -->
            <el-input-number
              v-else
              v-model="form.fail_threshold_value"
              :min="1"
              style="width: 100%"
            />
          </el-form-item>
        </el-card>

        <!-- 操作按钮 -->
        <div class="wizard-actions form-actions">
          <el-button @click="handleCancel">取消</el-button>
          <el-button type="primary" :loading="submitLoading" @click="handleSubmit">
            {{ isEditMode ? '保存修改' : '创建任务' }}
          </el-button>
        </div>
      </el-form>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Position, Monitor, Link } from '@element-plus/icons-vue'
import api from '../api'

// ==================== 路由与状态 ====================

const route = useRoute()
const router = useRouter()

// 判断是否为编辑模式：路由中存在 id 参数则为编辑模式
const isEditMode = computed(() => !!route.params.id)

// 页面加载状态
const pageLoading = ref(false)

// 表单提交加载状态
const submitLoading = ref(false)

// 表单引用
const formRef = ref(null)

// 凭证列表（用于下拉选择）
const credentials = ref([])

// ==================== 表单数据 ====================

const form = reactive({
  credential_id: null,
  domain: '',
  sub_domain: '',
  record_type: 'A',
  probe_protocol: '',
  probe_port: 80,
  probe_interval_sec: 60,
  timeout_ms: 3000,
  fail_threshold: 3,
  recover_threshold: 2,
  fail_threshold_type: 'count',
  fail_threshold_value: 1
})

// ==================== 计算属性 ====================

/**
 * 是否显示端口字段
 * 仅在 TCP/UDP/HTTP/HTTPS 协议时显示
 */
const showPortField = computed(() => {
  return ['TCP', 'UDP', 'HTTP', 'HTTPS'].includes(form.probe_protocol)
})

/**
 * 是否显示CNAME专用配置
 * 仅记录类型为CNAME时显示
 */
const showCnameConfig = computed(() => {
  return form.record_type === 'CNAME'
})

// ==================== 事件处理 ====================

/**
 * 协议切换时自动设置默认端口
 */
const onProtocolChange = () => {
  if (form.probe_protocol === 'HTTP') {
    form.probe_port = 80
  } else if (form.probe_protocol === 'HTTPS') {
    form.probe_port = 443
  } else if (form.probe_protocol === 'TCP' || form.probe_protocol === 'UDP') {
    if (!form.probe_port) {
      form.probe_port = 80
    }
  }
}

// ==================== 表单验证规则 ====================

/**
 * 正整数验证器
 */
const validatePositiveInt = (rule, value, callback) => {
  if (value === null || value === undefined || value === '') {
    callback(new Error('此字段为必填项'))
  } else if (!Number.isInteger(value) || value <= 0) {
    callback(new Error('请输入正整数'))
  } else {
    callback()
  }
}

const formRules = reactive({
  domain: [
    { required: true, message: '请输入域名', trigger: 'blur' }
  ],
  sub_domain: [
    { required: true, message: '请输入主机记录', trigger: 'blur' }
  ],
  record_type: [
    { required: true, message: '请选择记录类型', trigger: 'change' }
  ],
  probe_protocol: [
    { required: true, message: '请选择探测协议', trigger: 'change' }
  ],
  probe_interval_sec: [
    { required: true, validator: validatePositiveInt, trigger: 'blur' }
  ],
  timeout_ms: [
    { required: true, validator: validatePositiveInt, trigger: 'blur' }
  ],
  fail_threshold: [
    { required: true, validator: validatePositiveInt, trigger: 'blur' }
  ],
  recover_threshold: [
    { required: true, validator: validatePositiveInt, trigger: 'blur' }
  ],
  fail_threshold_type: [
    { required: true, message: '请选择阈值类型', trigger: 'change' }
  ],
  fail_threshold_value: [
    { required: true, validator: validatePositiveInt, trigger: 'blur' }
  ]
})

// ==================== API 调用 ====================

/**
 * 获取凭证列表（用于下拉选择）
 */
const fetchCredentials = async () => {
  try {
    const response = await api.get('/credentials')
    credentials.value = response.data
  } catch (error) {
    ElMessage.error('获取凭证列表失败')
  }
}

/**
 * 加载任务数据（编辑模式）
 */
const fetchTask = async () => {
  const taskId = route.params.id
  pageLoading.value = true
  try {
    const response = await api.get(`/health-monitors/${taskId}`)
    const task = response.data.data
    // 回填表单数据
    form.credential_id = task.credential_id || null
    form.domain = task.domain
    form.sub_domain = task.sub_domain
    form.record_type = task.record_type || 'A'
    form.probe_protocol = task.probe_protocol
    form.probe_port = task.probe_port || 80
    form.probe_interval_sec = task.probe_interval_sec
    form.timeout_ms = task.timeout_ms
    form.fail_threshold = task.fail_threshold
    form.recover_threshold = task.recover_threshold
    form.fail_threshold_type = task.fail_threshold_type || 'count'
    form.fail_threshold_value = task.fail_threshold_value || 1
  } catch (error) {
    ElMessage.error('获取任务数据失败')
    router.push('/health-monitors')
  } finally {
    pageLoading.value = false
  }
}

/**
 * 提交表单
 */
const handleSubmit = async () => {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitLoading.value = true

  // 构建请求数据
  const requestData = {
    credential_id: form.credential_id || null,
    domain: form.domain,
    sub_domain: form.sub_domain,
    record_type: form.record_type,
    probe_protocol: form.probe_protocol,
    probe_port: showPortField.value ? form.probe_port : 0,
    probe_interval_sec: form.probe_interval_sec,
    timeout_ms: form.timeout_ms,
    fail_threshold: form.fail_threshold,
    recover_threshold: form.recover_threshold,
    fail_threshold_type: showCnameConfig.value ? form.fail_threshold_type : 'count',
    fail_threshold_value: showCnameConfig.value ? form.fail_threshold_value : 1
  }

  try {
    if (isEditMode.value) {
      await api.put(`/health-monitors/${route.params.id}`, requestData)
      ElMessage.success('任务更新成功')
    } else {
      await api.post('/health-monitors', requestData)
      ElMessage.success('任务创建成功')
    }
    router.push('/health-monitors')
  } catch (error) {
    const action = isEditMode.value ? '更新' : '创建'
    ElMessage.error(`${action}任务失败`)
  } finally {
    submitLoading.value = false
  }
}

/**
 * 取消操作，返回健康监控列表
 */
const handleCancel = () => {
  router.push('/health-monitors')
}

// ==================== 生命周期 ====================

onMounted(async () => {
  await fetchCredentials()
  if (isEditMode.value) {
    await fetchTask()
  }
})
</script>

<style scoped>
/* 任务表单页面容器 */
.task-form-page {
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

/* 表单分组卡片 */
.form-section {
  margin-bottom: 20px;
}

/* 卡片分组标题 */
.section-header {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}

.section-header .el-icon {
  font-size: 18px;
  color: #409eff;
}

/* 表单操作按钮区域 */
.wizard-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.form-actions {
  margin-top: 24px;
}

/* 表单单位文字 */
.form-unit {
  margin-left: 8px;
  color: #909399;
  font-size: 14px;
}

/* 凭证提示文字 */
.credential-hint {
  margin-top: 4px;
  line-height: 1.4;
}

/* ==================== 响应式适配 ==================== */

/* 小屏幕：标签上方显示，减少左侧间距 */
@media screen and (max-width: 768px) {
  .task-form-page {
    padding: 12px;
  }

  .task-form-page :deep(.el-form-item__label) {
    width: auto !important;
    text-align: left;
    padding-bottom: 4px;
  }

  .task-form-page :deep(.el-form-item) {
    display: block;
  }

  .task-form-page :deep(.el-form-item__content) {
    margin-left: 0 !important;
  }

  .wizard-actions {
    flex-direction: column-reverse;
    gap: 10px;
  }

  .wizard-actions .el-button {
    width: 100%;
    margin-left: 0 !important;
  }
}
</style>
