<template>
  <!-- 解析池详情页面 -->
  <div class="pool-detail-page">
    <!-- 页面头部：池名称和返回按钮 -->
    <div class="page-header">
      <h2 class="page-title">解析池详情 - {{ pool.name }}</h2>
      <el-button @click="goBack">返回列表</el-button>
    </div>

    <!-- 基本信息卡片 -->
    <el-card class="info-card" shadow="never" v-loading="poolLoading">
      <template #header>
        <span class="card-title">基本信息</span>
      </template>
      <el-descriptions :column="3" border>
        <el-descriptions-item label="池名称">{{ pool.name }}</el-descriptions-item>
        <el-descriptions-item label="资源类型">{{ formatResourceType(pool.resource_type) }}</el-descriptions-item>
        <el-descriptions-item label="探测协议">{{ pool.probe_protocol }}</el-descriptions-item>
        <el-descriptions-item label="探测端口">{{ pool.probe_port || '-' }}</el-descriptions-item>
        <el-descriptions-item label="探测周期">{{ pool.probe_interval_sec }} 秒</el-descriptions-item>
        <el-descriptions-item label="超时时间">{{ pool.timeout_ms }} 毫秒</el-descriptions-item>
        <el-descriptions-item label="失败阈值">{{ pool.fail_threshold }}</el-descriptions-item>
        <el-descriptions-item label="恢复阈值">{{ pool.recover_threshold }}</el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ pool.created_at || '-' }}</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <!-- 健康摘要统计 -->
    <el-row :gutter="20" class="health-summary">
      <el-col :span="8">
        <el-card shadow="never" class="stat-card">
          <div class="stat-number healthy">{{ health.healthy }}</div>
          <div class="stat-label">健康资源</div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="never" class="stat-card">
          <div class="stat-number unhealthy">{{ health.unhealthy }}</div>
          <div class="stat-label">不健康资源</div>
        </el-card>
      </el-col>
      <el-col :span="8">
        <el-card shadow="never" class="stat-card">
          <div class="stat-number total">{{ health.total }}</div>
          <div class="stat-label">总资源数</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 资源列表 -->
    <el-card shadow="never" class="resource-card">
      <template #header>
        <div style="display: flex; justify-content: space-between; align-items: center">
          <span class="card-title">资源列表</span>
          <el-button type="primary" size="small" @click="openAddResourceDialog">添加资源</el-button>
        </div>
      </template>
      <el-table
        :data="resources"
        v-loading="resourcesLoading"
        border
        stripe
        style="width: 100%"
        empty-text="暂无资源"
      >
        <!-- 资源值列：域名类型支持点击查看IP -->
        <el-table-column label="资源值" min-width="180">
          <template #default="{ row }">
            <template v-if="pool.resource_type === 'domain'">
              <el-button link type="primary" @click="handleResolveDomain(row)">
                {{ row.value }}
              </el-button>
            </template>
            <template v-else>
              {{ row.value }}
            </template>
          </template>
        </el-table-column>

        <!-- 健康状态列 -->
        <el-table-column label="健康状态" min-width="120">
          <template #default="{ row }">
            <el-tag v-if="row.health_status === 'healthy'" type="success">健康</el-tag>
            <el-tag v-else-if="row.health_status === 'unhealthy'" type="danger">不健康</el-tag>
            <el-tag v-else type="info">未知</el-tag>
          </template>
        </el-table-column>

        <!-- 使用状态列 -->
        <el-table-column label="使用状态" min-width="140">
          <template #default="{ row }">
            <el-tooltip v-if="row.in_use" :content="'正在被 ' + row.in_use_by + ' 使用'" placement="top">
              <el-tag type="warning">使用中</el-tag>
            </el-tooltip>
            <el-tag v-else type="info">空闲</el-tag>
          </template>
        </el-table-column>

        <!-- 启用状态列 -->
        <el-table-column label="探测状态" min-width="100">
          <template #default="{ row }">
            <el-tag v-if="row.enabled" type="success">已启用</el-tag>
            <el-tag v-else type="info">已暂停</el-tag>
          </template>
        </el-table-column>

        <!-- 延迟列 -->
        <el-table-column label="延迟" min-width="100">
          <template #default="{ row }">
            {{ row.avg_latency_ms ? row.avg_latency_ms + ' ms' : '-' }}
          </template>
        </el-table-column>

        <!-- 最近探测时间列 -->
        <el-table-column label="最近探测时间" min-width="180">
          <template #default="{ row }">
            {{ row.last_probe_at || '-' }}
          </template>
        </el-table-column>

        <!-- 操作列 -->
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-button
              v-if="row.enabled"
              type="warning"
              size="small"
              @click="handleDisableResource(row)"
            >
              暂停
            </el-button>
            <el-button
              v-else
              type="success"
              size="small"
              @click="handleEnableResource(row)"
            >
              启动
            </el-button>
            <el-button type="danger" size="small" @click="handleRemoveResource(row)">
              移除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 添加资源对话框 -->
    <el-dialog
      v-model="addResourceDialogVisible"
      title="添加资源"
      width="480px"
      :close-on-click-modal="false"
      @closed="resetAddResourceForm"
    >
      <el-form
        ref="addResourceFormRef"
        :model="addResourceForm"
        :rules="addResourceFormRules"
        label-width="80px"
      >
        <el-form-item :label="pool.resource_type === 'domain' ? '域名' : 'IP 地址'" prop="value">
          <el-input
            v-model="addResourceForm.value"
            :placeholder="pool.resource_type === 'domain' ? '请输入域名，例如 example.com' : '请输入 IP 地址，例如 1.2.3.4'"
          />
        </el-form-item>
        <div class="add-resource-tip">
          <el-icon><InfoFilled /></el-icon>
          <span v-if="pool.resource_type === 'domain'">仅允许添加域名格式的资源</span>
          <span v-else>仅允许添加 IP 地址格式的资源（支持 IPv4 和 IPv6）</span>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="addResourceDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="addResourceLoading" @click="handleAddResourceSubmit">
          确定添加
        </el-button>
      </template>
    </el-dialog>

    <!-- 域名解析IP对话框 -->
    <el-dialog
      v-model="resolveDialogVisible"
      :title="'域名解析 - ' + resolveResult.domain"
      width="620px"
    >
      <div v-loading="resolveLoading">
        <div v-if="resolveResult.cachedAt" class="resolve-cache-info">
          <el-icon><Clock /></el-icon>
          <span>{{ resolveResult.cached ? '缓存数据' : '最新数据' }}，探测时间：{{ resolveResult.cachedAt }}</span>
        </div>
        <el-alert
          v-if="resolveResult.error"
          :title="resolveResult.error"
          type="warning"
          show-icon
          :closable="false"
          style="margin-bottom: 16px"
        />
        <el-table
          v-if="resolveResult.ips.length > 0"
          :data="resolveResult.ips"
          border
          stripe
          style="width: 100%"
        >
          <el-table-column label="IP 地址" prop="ip" min-width="160" />
          <el-table-column label="探测状态" min-width="100">
            <template #default="{ row }">
              <el-tag v-if="row.success" type="success">正常</el-tag>
              <el-tag v-else type="danger">异常</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="延迟" min-width="100">
            <template #default="{ row }">
              {{ row.success ? row.latency_ms + ' ms' : '-' }}
            </template>
          </el-table-column>
          <el-table-column label="错误信息" min-width="180" show-overflow-tooltip>
            <template #default="{ row }">
              {{ row.error || '-' }}
            </template>
          </el-table-column>
        </el-table>
        <el-empty v-else-if="!resolveLoading && !resolveResult.error" description="未解析到任何 IP 地址" />
      </div>
      <template #footer>
        <el-button @click="resolveDialogVisible = false">关闭</el-button>
        <el-button type="primary" :loading="resolveLoading" @click="handleRefreshResolve">
          刷新探测
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import { InfoFilled, Clock } from '@element-plus/icons-vue'
import api from '../api'

// ==================== 路由 ====================

const route = useRoute()
const router = useRouter()

// 解析池 ID
const poolId = route.params.id

// ==================== 状态定义 ====================

// 解析池基本信息
const pool = reactive({
  id: null,
  name: '',
  resource_type: '',
  probe_protocol: '',
  probe_port: 0,
  probe_interval_sec: 0,
  timeout_ms: 0,
  fail_threshold: 0,
  recover_threshold: 0,
  created_at: ''
})
const poolLoading = ref(false)

// 健康摘要信息
const health = reactive({
  total: 0,
  healthy: 0,
  unhealthy: 0
})

// 资源列表
const resources = ref([])
const resourcesLoading = ref(false)

// 添加资源对话框
const addResourceDialogVisible = ref(false)
const addResourceLoading = ref(false)
const addResourceFormRef = ref(null)
const addResourceForm = reactive({
  value: ''
})

// 域名解析IP对话框
const resolveDialogVisible = ref(false)
const resolveLoading = ref(false)
const currentResolveResource = ref(null)
const resolveResult = reactive({
  domain: '',
  ips: [],
  error: '',
  cachedAt: '',
  cached: false
})

// ==================== 表单验证规则 ====================

/**
 * IP 地址格式验证
 */
const validateIP = (rule, value, callback) => {
  if (!value) {
    callback(new Error('请输入 IP 地址'))
    return
  }
  // IPv4 正则
  const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/
  // IPv6 简单正则
  const ipv6Regex = /^([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}$/
  if (!ipv4Regex.test(value) && !ipv6Regex.test(value)) {
    callback(new Error('请输入有效的 IP 地址（支持 IPv4 和 IPv6）'))
    return
  }
  if (ipv4Regex.test(value)) {
    const parts = value.split('.')
    if (parts.some(p => parseInt(p) > 255)) {
      callback(new Error('IPv4 地址每段不能超过 255'))
      return
    }
  }
  callback()
}

/**
 * 域名格式验证
 */
const validateDomain = (rule, value, callback) => {
  if (!value) {
    callback(new Error('请输入域名'))
    return
  }
  const domainRegex = /^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}$/
  if (!domainRegex.test(value)) {
    callback(new Error('请输入有效的域名格式，例如 example.com'))
    return
  }
  callback()
}

const addResourceFormRules = reactive({
  value: [
    {
      required: true,
      validator: (rule, value, callback) => {
        if (pool.resource_type === 'domain') {
          validateDomain(rule, value, callback)
        } else {
          validateIP(rule, value, callback)
        }
      },
      trigger: 'blur'
    }
  ]
})

// ==================== 辅助函数 ====================

const formatResourceType = (type) => {
  if (type === 'ip') return 'IP'
  if (type === 'domain') return '域名'
  return type || '-'
}

const goBack = () => {
  router.push('/pools')
}

// ==================== API 调用 ====================

const fetchPool = async () => {
  poolLoading.value = true
  try {
    const response = await api.get(`/pools/${poolId}`)
    Object.assign(pool, response.data)
  } catch (error) {
    ElMessage.error('获取解析池信息失败')
  } finally {
    poolLoading.value = false
  }
}

const fetchResources = async () => {
  resourcesLoading.value = true
  try {
    const response = await api.get(`/pools/${poolId}/resources`)
    resources.value = response.data || []
  } catch (error) {
    ElMessage.error('获取资源列表失败')
  } finally {
    resourcesLoading.value = false
  }
}

const fetchHealth = async () => {
  try {
    const response = await api.get(`/pools/${poolId}/health`)
    Object.assign(health, response.data)
  } catch (error) {
    ElMessage.error('获取健康摘要失败')
  }
}

// ==================== 添加资源 ====================

const openAddResourceDialog = () => {
  addResourceDialogVisible.value = true
}

const resetAddResourceForm = () => {
  if (addResourceFormRef.value) {
    addResourceFormRef.value.resetFields()
  }
  addResourceForm.value = ''
}

const handleAddResourceSubmit = async () => {
  if (!addResourceFormRef.value) return
  const valid = await addResourceFormRef.value.validate().catch(() => false)
  if (!valid) return

  addResourceLoading.value = true
  try {
    await api.post(`/pools/${poolId}/resources`, { value: addResourceForm.value })
    ElMessage.success('资源添加成功')
    addResourceDialogVisible.value = false
    await Promise.all([fetchResources(), fetchHealth()])
  } catch (error) {
    if (error.response && error.response.data && error.response.data.error) {
      ElMessage.error(error.response.data.error)
    } else {
      ElMessage.error('添加资源失败')
    }
  } finally {
    addResourceLoading.value = false
  }
}

// ==================== 移除资源 ====================

const handleRemoveResource = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要移除资源「${row.value}」吗？移除后不可恢复。`,
      '移除确认',
      {
        confirmButtonText: '确定移除',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    try {
      await api.delete(`/pools/${poolId}/resources/${row.id}`)
      ElMessage.success('资源已移除')
      await Promise.all([fetchResources(), fetchHealth()])
    } catch (error) {
      if (error.response && error.response.data && error.response.data.error) {
        ElMessage.error(error.response.data.error)
      } else {
        ElMessage.error('移除资源失败')
      }
    }
  } catch {
    // 用户取消
  }
}

// ==================== 启用/禁用资源 ====================

/**
 * 启用资源探测
 */
const handleEnableResource = async (row) => {
  try {
    await api.put(`/pools/${poolId}/resources/${row.id}/enable`)
    ElMessage.success('资源已启用')
    await fetchResources()
  } catch (error) {
    if (error.response && error.response.data && error.response.data.error) {
      ElMessage.error(error.response.data.error)
    } else {
      ElMessage.error('启用资源失败')
    }
  }
}

/**
 * 禁用资源探测
 */
const handleDisableResource = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要暂停资源「${row.value}」的探测吗？`,
      '暂停确认',
      {
        confirmButtonText: '确定暂停',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    try {
      await api.put(`/pools/${poolId}/resources/${row.id}/disable`)
      ElMessage.success('资源已暂停')
      await fetchResources()
    } catch (error) {
      if (error.response && error.response.data && error.response.data.error) {
        ElMessage.error(error.response.data.error)
      } else {
        ElMessage.error('暂停资源失败')
      }
    }
  } catch {
    // 用户取消
  }
}

// ==================== 域名解析IP ====================

/**
 * 解析域名下的所有IP地址（使用缓存）
 */
const handleResolveDomain = async (row) => {
  if (!row) return
  currentResolveResource.value = row
  resolveDialogVisible.value = true
  await fetchResolveData(row, false)
}

/**
 * 强制刷新域名解析
 */
const handleRefreshResolve = async () => {
  if (!currentResolveResource.value) return
  await fetchResolveData(currentResolveResource.value, true)
}

/**
 * 获取域名解析数据
 * @param {Object} row - 资源行数据
 * @param {boolean} refresh - 是否强制刷新
 */
const fetchResolveData = async (row, refresh) => {
  resolveLoading.value = true
  resolveResult.domain = row.value
  resolveResult.ips = []
  resolveResult.error = ''
  resolveResult.cachedAt = ''
  resolveResult.cached = false

  try {
    const params = refresh ? { refresh: 'true' } : {}
    const response = await api.get(`/pools/${poolId}/resources/${row.id}/resolve`, { params })
    const data = response.data
    resolveResult.domain = data.domain || row.value
    resolveResult.ips = data.ips || []
    resolveResult.error = data.error || ''
    resolveResult.cachedAt = data.cached_at || ''
    resolveResult.cached = data.cached || false
  } catch (error) {
    resolveResult.error = '请求失败，请稍后重试'
  } finally {
    resolveLoading.value = false
  }
}

// ==================== 生命周期 ====================

onMounted(() => {
  fetchPool()
  fetchResources()
  fetchHealth()
})
</script>

<style scoped>
.pool-detail-page {
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.page-title {
  margin: 0;
  font-size: 20px;
  color: #303133;
  font-weight: 600;
}

.info-card {
  margin-bottom: 20px;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.health-summary {
  margin-bottom: 20px;
}

.stat-card {
  text-align: center;
  padding: 10px 0;
}

.stat-number {
  font-size: 32px;
  font-weight: 700;
  line-height: 1.2;
}

.stat-number.healthy {
  color: #67c23a;
}

.stat-number.unhealthy {
  color: #f56c6c;
}

.stat-number.total {
  color: #409eff;
}

.stat-label {
  font-size: 14px;
  color: #909399;
  margin-top: 8px;
}

.resource-card {
  margin-bottom: 20px;
}

/* 添加资源提示 */
.add-resource-tip {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  background: #f0f9eb;
  border-radius: 4px;
  font-size: 13px;
  color: #67c23a;
  margin-top: 4px;
}

.add-resource-tip .el-icon {
  font-size: 14px;
}

/* 域名解析缓存信息 */
.resolve-cache-info {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  background: #f4f4f5;
  border-radius: 4px;
  font-size: 13px;
  color: #909399;
  margin-bottom: 12px;
}

.resolve-cache-info .el-icon {
  font-size: 14px;
}
</style>
