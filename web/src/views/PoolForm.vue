<template>
  <!-- 解析池表单页面：创建模式使用分步引导，编辑模式直接显示表单 -->
  <div class="pool-form-page">
    <!-- 页面头部 -->
    <div class="page-header">
      <h2 class="page-title">{{ isEditMode ? '编辑解析池' : '创建解析池' }}</h2>
    </div>

    <!-- 创建模式：分步引导 -->
    <div v-if="!isEditMode" v-loading="pageLoading">
      <!-- 步骤条 -->
      <el-steps :active="currentStep" finish-status="success" align-center class="wizard-steps">
        <el-step title="资源类型" description="选择池中资源的类型" />
        <el-step title="池配置" description="填写详细参数" />
      </el-steps>

      <!-- 步骤 1：选择资源类型 -->
      <div v-if="currentStep === 0" class="wizard-content">
        <h3 class="step-title">请选择资源类型</h3>
        <p class="step-desc">资源类型决定了解析池中存储的备用资源种类，创建后不可修改。</p>
        <div class="type-cards">
          <div
            class="type-card"
            :class="{ active: form.resource_type === 'ip' }"
            @click="form.resource_type = 'ip'"
          >
            <div class="type-card-icon"><el-icon><Position /></el-icon></div>
            <div class="type-card-title">IP 地址</div>
            <div class="type-card-desc">
              池中存储 IP 地址（支持 IPv4 和 IPv6）。当故障转移触发时，系统将从池中选择健康的 IP 地址替换当前解析记录。适用于直接管理服务器 IP 的场景。
            </div>
          </div>
          <div
            class="type-card"
            :class="{ active: form.resource_type === 'domain' }"
            @click="form.resource_type = 'domain'"
          >
            <div class="type-card-icon"><el-icon><Link /></el-icon></div>
            <div class="type-card-title">域名</div>
            <div class="type-card-desc">
              池中存储域名地址。当故障转移触发时，系统将从池中选择健康的域名替换当前解析记录。适用于使用 CNAME 或需要通过域名间接管理的场景。
            </div>
          </div>
        </div>
        <div class="wizard-actions">
          <el-button @click="handleCancel">取消</el-button>
          <el-button type="primary" @click="currentStep = 1">下一步</el-button>
        </div>
      </div>

      <!-- 步骤 2：池配置表单 -->
      <div v-if="currentStep === 1" class="wizard-content wizard-form-content">
        <el-form ref="formRef" :model="form" :rules="formRules" label-width="120px">
          <!-- 已选配置摘要 -->
          <div class="config-summary">
            <el-tag type="primary" effect="dark" size="large">
              {{ form.resource_type === 'ip' ? 'IP 地址' : '域名' }}
            </el-tag>
            <el-button link type="primary" class="summary-change" @click="currentStep = 0">修改选择</el-button>
          </div>

          <!-- 基本信息 -->
          <el-card class="form-section" shadow="never">
            <template #header>
              <div class="section-header">
                <el-icon><Collection /></el-icon>
                <span>基本信息</span>
              </div>
            </template>
            <el-form-item label="池名称" prop="name">
              <el-input v-model="form.name" placeholder="请输入解析池名称" />
            </el-form-item>
            <el-form-item label="描述">
              <el-input v-model="form.description" type="textarea" :rows="2" placeholder="可选，描述该解析池的用途" />
            </el-form-item>
          </el-card>

          <!-- 探测配置 -->
          <el-card class="form-section" shadow="never">
            <template #header>
              <div class="section-header">
                <el-icon><Monitor /></el-icon>
                <span>探测配置</span>
              </div>
            </template>
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="探测协议" prop="probe_protocol">
                  <el-select v-model="form.probe_protocol" placeholder="请选择" style="width: 100%" @change="onProtocolChange">
                    <el-option label="ICMP" value="ICMP" />
                    <el-option label="TCP" value="TCP" />
                    <el-option label="UDP" value="UDP" />
                    <el-option label="HTTP" value="HTTP" />
                    <el-option label="HTTPS" value="HTTPS" />
                  </el-select>
                </el-form-item>
              </el-col>
              <el-col v-if="showPortField" :span="12">
                <el-form-item label="探测端口" prop="probe_port">
                  <el-input-number v-model="form.probe_port" :min="1" :max="65535" style="width: 100%" />
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="探测周期" prop="probe_interval_sec">
                  <el-input-number v-model="form.probe_interval_sec" :min="1" style="width: 100%" />
                  <span class="form-unit">秒</span>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="超时时间" prop="timeout_ms">
                  <el-input-number v-model="form.timeout_ms" :min="100" style="width: 100%" />
                  <span class="form-unit">毫秒</span>
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="20">
              <el-col :span="12">
                <el-form-item label="失败阈值" prop="fail_threshold">
                  <el-tooltip content="连续探测失败达到此次数后，标记资源为不健康" placement="top">
                    <el-input-number v-model="form.fail_threshold" :min="1" style="width: 100%" />
                  </el-tooltip>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="恢复阈值" prop="recover_threshold">
                  <el-tooltip content="连续探测成功达到此次数后，标记资源为健康" placement="top">
                    <el-input-number v-model="form.recover_threshold" :min="1" style="width: 100%" />
                  </el-tooltip>
                </el-form-item>
              </el-col>
            </el-row>
          </el-card>

          <!-- 操作按钮 -->
          <div class="wizard-actions form-actions">
            <div>
              <el-button @click="currentStep = 0">上一步</el-button>
              <el-button @click="handleCancel">取消</el-button>
            </div>
            <el-button type="primary" :loading="submitLoading" @click="handleSubmit">创建解析池</el-button>
          </div>
        </el-form>
      </div>
    </div>

    <!-- 编辑模式：直接显示表单 -->
    <div v-if="isEditMode" class="wizard-content wizard-form-content" v-loading="pageLoading">
      <el-form ref="formRef" :model="form" :rules="formRules" label-width="120px">
        <!-- 已选配置摘要（只读） -->
        <div class="config-summary">
          <el-tag type="primary" effect="dark" size="large">
            {{ form.resource_type === 'ip' ? 'IP 地址' : '域名' }}
          </el-tag>
          <el-tag type="info" size="small" style="margin-left: 8px">资源类型不可修改</el-tag>
        </div>

        <!-- 基本信息 -->
        <el-card class="form-section" shadow="never">
          <template #header>
            <div class="section-header">
              <el-icon><Collection /></el-icon>
              <span>基本信息</span>
            </div>
          </template>
          <el-form-item label="池名称">
            <el-input v-model="form.name" disabled />
          </el-form-item>
          <el-form-item label="描述">
            <el-input v-model="form.description" type="textarea" :rows="2" placeholder="可选，描述该解析池的用途" />
          </el-form-item>
        </el-card>

        <!-- 探测配置 -->
        <el-card class="form-section" shadow="never">
          <template #header>
            <div class="section-header">
              <el-icon><Monitor /></el-icon>
              <span>探测配置</span>
            </div>
          </template>
          <el-row :gutter="20">
            <el-col :span="12">
              <el-form-item label="探测协议" prop="probe_protocol">
                <el-select v-model="form.probe_protocol" placeholder="请选择" style="width: 100%" @change="onProtocolChange">
                  <el-option label="ICMP" value="ICMP" />
                  <el-option label="TCP" value="TCP" />
                  <el-option label="UDP" value="UDP" />
                  <el-option label="HTTP" value="HTTP" />
                  <el-option label="HTTPS" value="HTTPS" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col v-if="showPortField" :span="12">
              <el-form-item label="探测端口" prop="probe_port">
                <el-input-number v-model="form.probe_port" :min="1" :max="65535" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="20">
            <el-col :span="12">
              <el-form-item label="探测周期" prop="probe_interval_sec">
                <el-input-number v-model="form.probe_interval_sec" :min="1" style="width: 100%" />
                <span class="form-unit">秒</span>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="超时时间" prop="timeout_ms">
                <el-input-number v-model="form.timeout_ms" :min="100" style="width: 100%" />
                <span class="form-unit">毫秒</span>
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="20">
            <el-col :span="12">
              <el-form-item label="失败阈值" prop="fail_threshold">
                <el-tooltip content="连续探测失败达到此次数后，标记资源为不健康" placement="top">
                  <el-input-number v-model="form.fail_threshold" :min="1" style="width: 100%" />
                </el-tooltip>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="恢复阈值" prop="recover_threshold">
                <el-tooltip content="连续探测成功达到此次数后，标记资源为健康" placement="top">
                  <el-input-number v-model="form.recover_threshold" :min="1" style="width: 100%" />
                </el-tooltip>
              </el-form-item>
            </el-col>
          </el-row>
        </el-card>

        <!-- 操作按钮 -->
        <div class="wizard-actions form-actions">
          <el-button @click="handleCancel">取消</el-button>
          <el-button type="primary" :loading="submitLoading" @click="handleSubmit">保存修改</el-button>
        </div>
      </el-form>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Position, Link, Collection, Monitor } from '@element-plus/icons-vue'
import api from '../api'

// ==================== 路由与状态 ====================

const route = useRoute()
const router = useRouter()

// 判断是否为编辑模式
const isEditMode = computed(() => !!route.params.id)

// 当前引导步骤（0: 资源类型, 1: 表单配置）
const currentStep = ref(0)

// 页面加载状态
const pageLoading = ref(false)

// 表单提交加载状态
const submitLoading = ref(false)

// 表单引用
const formRef = ref(null)

// ==================== 表单数据 ====================

const form = reactive({
  name: '',
  resource_type: 'ip',
  description: '',
  probe_protocol: '',
  probe_port: 80,
  probe_interval_sec: 60,
  timeout_ms: 3000,
  fail_threshold: 3,
  recover_threshold: 3
})

// ==================== 计算属性 ====================

/**
 * 是否显示端口字段
 */
const showPortField = computed(() => {
  return ['TCP', 'UDP', 'HTTP', 'HTTPS'].includes(form.probe_protocol)
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
  name: [
    { required: true, message: '请输入池名称', trigger: 'blur' }
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
  ]
})

// ==================== API 调用 ====================

/**
 * 加载解析池数据（编辑模式）
 */
const fetchPool = async () => {
  const poolId = route.params.id
  pageLoading.value = true
  try {
    const response = await api.get(`/pools/${poolId}`)
    const pool = response.data
    form.name = pool.name
    form.resource_type = pool.resource_type
    form.description = pool.description || ''
    form.probe_protocol = pool.probe_protocol
    form.probe_port = pool.probe_port || 80
    form.probe_interval_sec = pool.probe_interval_sec
    form.timeout_ms = pool.timeout_ms
    form.fail_threshold = pool.fail_threshold
    form.recover_threshold = pool.recover_threshold
  } catch (error) {
    ElMessage.error('获取解析池数据失败')
    router.push('/pools')
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

  const requestData = {
    name: form.name,
    resource_type: form.resource_type,
    description: form.description,
    probe_protocol: form.probe_protocol,
    probe_port: showPortField.value ? form.probe_port : 0,
    probe_interval_sec: form.probe_interval_sec,
    timeout_ms: form.timeout_ms,
    fail_threshold: form.fail_threshold,
    recover_threshold: form.recover_threshold
  }

  try {
    if (isEditMode.value) {
      await api.put(`/pools/${route.params.id}`, requestData)
      ElMessage.success('解析池更新成功')
    } else {
      await api.post('/pools', requestData)
      ElMessage.success('解析池创建成功')
    }
    router.push('/pools')
  } catch (error) {
    const action = isEditMode.value ? '更新' : '创建'
    if (error.response && error.response.data && error.response.data.error) {
      ElMessage.error(error.response.data.error)
    } else {
      ElMessage.error(`${action}解析池失败`)
    }
  } finally {
    submitLoading.value = false
  }
}

/**
 * 取消操作，返回解析池列表
 */
const handleCancel = () => {
  router.push('/pools')
}

// ==================== 生命周期 ====================

onMounted(async () => {
  if (isEditMode.value) {
    await fetchPool()
  }
})
</script>

<style scoped>
/* 解析池表单页面容器 */
.pool-form-page {
  padding: 20px;
}

/* 页面头部 */
.page-header {
  margin-bottom: 20px;
}

.page-title {
  margin: 0;
  font-size: 20px;
  color: #303133;
  font-weight: 600;
}

/* 步骤条 */
.wizard-steps {
  margin-bottom: 32px;
}

/* 步骤标题 */
.step-title {
  margin: 0 0 8px 0;
  font-size: 18px;
  color: #303133;
  font-weight: 600;
}

/* 步骤说明文字 */
.step-desc {
  margin: 0 0 24px 0;
  font-size: 14px;
  color: #909399;
  line-height: 1.6;
}

/* 类型选择卡片容器 */
.type-cards {
  display: flex;
  gap: 20px;
  margin-bottom: 28px;
}

/* 类型选择卡片 */
.type-card {
  flex: 1;
  padding: 24px;
  border: 2px solid #e4e7ed;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
  background: #fff;
}

.type-card:hover {
  border-color: #c0c4cc;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.06);
}

.type-card.active {
  border-color: #409eff;
  background: #ecf5ff;
  box-shadow: 0 2px 12px rgba(64, 158, 255, 0.15);
}

.type-card-icon {
  margin-bottom: 12px;
}

.type-card-icon .el-icon {
  font-size: 32px;
  color: #909399;
}

.type-card.active .type-card-icon .el-icon {
  color: #409eff;
}

.type-card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 8px;
}

.type-card-desc {
  font-size: 13px;
  color: #606266;
  line-height: 1.6;
}

/* 引导操作按钮区域 */
.wizard-actions {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

/* 已选配置摘要栏 */
.config-summary {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 24px;
  padding: 12px 16px;
  background: #f5f7fa;
  border-radius: 8px;
}

.summary-change {
  margin-left: auto;
  font-size: 13px;
}

/* 表单分组卡片 */
.form-section {
  margin-bottom: 20px;
}

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

.form-actions {
  margin-top: 24px;
}

.form-unit {
  margin-left: 8px;
  color: #909399;
  font-size: 14px;
}
</style>
