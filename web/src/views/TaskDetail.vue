<template>
  <!-- 任务详情页面 -->
  <div class="task-detail-page">
    <!-- 页面头部：域名信息和返回按钮 -->
    <div class="page-header">
      <h2 class="page-title">
        任务详情 - {{ taskDomainDisplay }}
      </h2>
      <el-button @click="goBack">返回列表</el-button>
    </div>

    <!-- 任务基本信息 -->
    <el-card class="info-card" shadow="never" v-loading="taskLoading">
      <template #header>
        <span class="card-title">基本信息</span>
      </template>
      <el-descriptions :column="3" border>
        <el-descriptions-item label="域名">{{ task.domain }}</el-descriptions-item>
        <el-descriptions-item label="主机记录">{{ task.sub_domain }}</el-descriptions-item>
        <el-descriptions-item label="探测协议">{{ task.probe_protocol }}</el-descriptions-item>
        <el-descriptions-item label="探测端口">{{ task.probe_port || '-' }}</el-descriptions-item>
        <el-descriptions-item label="探测周期">{{ task.probe_interval_sec }} 秒</el-descriptions-item>
        <el-descriptions-item label="超时时间">{{ task.timeout_ms }} 毫秒</el-descriptions-item>
        <el-descriptions-item label="失败阈值">{{ task.fail_threshold }}</el-descriptions-item>
        <el-descriptions-item label="恢复阈值">{{ task.recover_threshold }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag v-if="task.enabled" type="success">运行中</el-tag>
          <el-tag v-else type="info">已停止</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="任务类型">
          {{ task.task_type === 'switch' ? '切换解析' : '暂停/删除' }}
        </el-descriptions-item>
        <el-descriptions-item label="记录类型">
          {{ task.record_type === 'CNAME' ? 'CNAME' : 'A/AAAA' }}
        </el-descriptions-item>
        <el-descriptions-item label="关联解析池">
          {{ task.pool_name || '-' }}
        </el-descriptions-item>
        <el-descriptions-item label="回切策略">
          {{ task.task_type === 'switch' ? (task.switch_back_policy === 'auto' ? '自动回切' : '保持当前') : '-' }}
        </el-descriptions-item>
      </el-descriptions>
    </el-card>

    <!-- 切换状态卡片：仅在已切换状态时显示 -->
    <el-card v-if="task.is_switched" class="switch-card" shadow="never">
      <template #header>
        <div style="display: flex; justify-content: space-between; align-items: center">
          <span class="card-title">切换状态</span>
          <el-tag type="danger">已切换</el-tag>
        </div>
      </template>
      <el-descriptions :column="2" border>
        <el-descriptions-item label="原始值">{{ task.original_value || '-' }}</el-descriptions-item>
        <el-descriptions-item label="当前值">{{ task.current_value || '-' }}</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <!-- 标签页：探测历史 / 操作日志 -->
    <el-card class="tabs-card" shadow="never">
      <el-tabs v-model="activeTab" @tab-change="handleTabChange">
        <!-- 探测历史标签页 -->
        <el-tab-pane label="探测历史" name="history">
          <!-- 筛选栏：IP + 状态 -->
          <div class="filter-bar">
            <el-input
              v-model="historyIpFilter"
              placeholder="输入 IP 地址筛选"
              clearable
              style="width: 220px; margin-right: 12px"
              @clear="fetchHistory"
            >
              <template #append>
                <el-button @click="fetchHistory">搜索</el-button>
              </template>
            </el-input>
            <el-select
              v-model="historyStatusFilter"
              placeholder="状态筛选"
              clearable
              style="width: 140px"
              @change="fetchHistory"
            >
              <el-option label="成功" value="true" />
              <el-option label="失败" value="false" />
            </el-select>
          </div>

          <!-- 探测历史表格 -->
          <el-table
            :data="historyData"
            v-loading="historyLoading"
            border
            stripe
            style="width: 100%"
            empty-text="暂无探测历史"
          >
            <el-table-column prop="ip" label="IP" min-width="140" />
            <el-table-column label="状态" min-width="80">
              <template #default="{ row }">
                <el-tag v-if="row.success" type="success">成功</el-tag>
                <el-tag v-else type="danger">失败</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="延迟" min-width="100">
              <template #default="{ row }">
                {{ row.latency_ms !== undefined && row.latency_ms !== null ? row.latency_ms + ' ms' : '-' }}
              </template>
            </el-table-column>
            <el-table-column prop="error_msg" label="错误信息" min-width="200" show-overflow-tooltip />
            <el-table-column prop="probed_at" label="探测时间" min-width="180" />
          </el-table>

          <!-- 探测历史分页 -->
          <div class="pagination-bar">
            <el-pagination
              v-model:current-page="historyPage"
              v-model:page-size="historyPageSize"
              :total="historyTotal"
              :page-sizes="[10, 20, 50, 100]"
              layout="total, sizes, prev, pager, next, jumper"
              @current-change="fetchHistory"
              @size-change="fetchHistory"
            />
          </div>
        </el-tab-pane>

        <!-- 操作日志标签页 -->
        <el-tab-pane label="操作日志" name="logs">
          <!-- 筛选栏：操作类型 + 时间范围 + IP + 状态 -->
          <div class="filter-bar">
            <el-select
              v-model="logsOperationType"
              placeholder="操作类型"
              clearable
              style="width: 140px; margin-right: 12px"
              @change="fetchLogs"
            >
              <el-option label="暂停" value="pause" />
              <el-option label="删除" value="delete" />
              <el-option label="恢复" value="resume" />
              <el-option label="添加" value="add" />
              <el-option label="切换" value="switch" />
            </el-select>
            <el-date-picker
              v-model="logsStartTime"
              type="datetime"
              placeholder="开始时间"
              clearable
              style="width: 200px; margin-right: 12px"
              @change="fetchLogs"
            />
            <el-date-picker
              v-model="logsEndTime"
              type="datetime"
              placeholder="结束时间"
              clearable
              style="width: 200px; margin-right: 12px"
              @change="fetchLogs"
            />
            <el-input
              v-model="logsIpFilter"
              placeholder="输入 IP 地址筛选"
              clearable
              style="width: 220px; margin-right: 12px"
              @clear="fetchLogs"
            >
              <template #append>
                <el-button @click="fetchLogs">搜索</el-button>
              </template>
            </el-input>
            <el-select
              v-model="logsStatusFilter"
              placeholder="状态筛选"
              clearable
              style="width: 140px"
              @change="fetchLogs"
            >
              <el-option label="成功" value="true" />
              <el-option label="失败" value="false" />
            </el-select>
          </div>

          <!-- 操作日志表格 -->
          <el-table
            :data="logsData"
            v-loading="logsLoading"
            border
            stripe
            style="width: 100%"
            empty-text="暂无操作日志"
          >
            <el-table-column label="操作类型" min-width="100">
              <template #default="{ row }">
                {{ operationTypeLabel(row.operation_type) }}
              </template>
            </el-table-column>
            <el-table-column prop="ip" label="IP" min-width="140" />
            <el-table-column prop="record_type" label="记录类型" min-width="80" />
            <el-table-column label="操作结果" min-width="80">
              <template #default="{ row }">
                <el-tag v-if="row.success" type="success">成功</el-tag>
                <el-tag v-else type="danger">失败</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="detail" label="详情" min-width="200" show-overflow-tooltip />
            <el-table-column prop="operated_at" label="操作时间" min-width="180" />
          </el-table>

          <!-- 操作日志分页 -->
          <div class="pagination-bar">
            <el-pagination
              v-model:current-page="logsPage"
              v-model:page-size="logsPageSize"
              :total="logsTotal"
              :page-sizes="[10, 20, 50, 100]"
              layout="total, sizes, prev, pager, next, jumper"
              @current-change="fetchLogs"
              @size-change="fetchLogs"
            />
          </div>
        </el-tab-pane>

        <!-- CNAME 信息标签页：仅CNAME记录类型显示 -->
        <el-tab-pane v-if="task.record_type === 'CNAME'" label="CNAME 信息" name="cname">
          <div v-loading="cnameLoading">
            <!-- 阈值配置信息 -->
            <el-descriptions :column="2" border class="cname-threshold-info">
              <el-descriptions-item label="阈值类型">
                {{ cnameInfo.threshold_type === 'percent' ? '百分比' : '个数' }}
              </el-descriptions-item>
              <el-descriptions-item label="阈值数值">{{ cnameInfo.threshold }}</el-descriptions-item>
              <el-descriptions-item label="失败计数">{{ cnameInfo.failed_count }}</el-descriptions-item>
            </el-descriptions>

            <!-- 目标IP列表 -->
            <el-table
              :data="cnameInfo.targets"
              border
              stripe
              style="width: 100%; margin-top: 16px"
              empty-text="暂无CNAME目标IP"
            >
              <el-table-column prop="ip" label="目标IP" min-width="150" />
              <el-table-column label="健康状态" min-width="120">
                <template #default="{ row }">
                  <el-tag v-if="row.health_status === 'healthy'" type="success">健康</el-tag>
                  <el-tag v-else-if="row.health_status === 'unhealthy'" type="danger">不健康</el-tag>
                  <el-tag v-else type="info">未知</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="延迟" min-width="100">
                <template #default="{ row }">
                  {{ row.latency_ms !== undefined && row.latency_ms !== null ? row.latency_ms + ' ms' : '-' }}
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>

        <!-- IP 管理标签页 -->
        <el-tab-pane label="IP 管理" name="ips">
          <el-table
            :data="ipsData"
            v-loading="ipsLoading"
            border
            stripe
            style="width: 100%"
            empty-text="暂无解析 IP"
          >
            <el-table-column prop="ip" label="IP 地址" min-width="150" />
            <el-table-column label="探测状态" min-width="100">
              <template #default="{ row }">
                <template v-if="row.excluded">
                  <el-tag type="warning">已排除</el-tag>
                </template>
                <template v-else-if="row.success === null || row.success === undefined">
                  <el-tag type="info">未探测</el-tag>
                </template>
                <template v-else>
                  <el-tag v-if="row.success" type="success">正常</el-tag>
                  <el-tag v-else type="danger">异常</el-tag>
                </template>
              </template>
            </el-table-column>
            <el-table-column label="延迟" min-width="100">
              <template #default="{ row }">
                {{ row.latency_ms ? row.latency_ms + ' ms' : '-' }}
              </template>
            </el-table-column>
            <el-table-column prop="last_probe" label="最近探测时间" min-width="180">
              <template #default="{ row }">
                {{ row.last_probe || '-' }}
              </template>
            </el-table-column>
            <el-table-column label="操作" width="160" fixed="right">
              <template #default="{ row }">
                <el-button
                  v-if="!row.excluded"
                  type="warning"
                  size="small"
                  @click="handleExcludeIP(row)"
                >
                  排除探测
                </el-button>
                <el-button
                  v-else
                  type="success"
                  size="small"
                  @click="handleIncludeIP(row)"
                >
                  恢复探测
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

// ==================== 路由 ====================

const route = useRoute()
const router = useRouter()

// 任务 ID，从路由参数获取
const taskId = route.params.id

// ==================== 状态定义 ====================

// 任务基本信息
const task = reactive({
  id: null,
  domain: '',
  sub_domain: '',
  probe_protocol: '',
  probe_port: 0,
  probe_interval_sec: 0,
  timeout_ms: 0,
  fail_threshold: 0,
  recover_threshold: 0,
  enabled: false,
  task_type: '',
  record_type: '',
  pool_name: '',
  switch_back_policy: '',
  is_switched: false,
  original_value: '',
  current_value: ''
})
const taskLoading = ref(false)

// 当前激活的标签页
const activeTab = ref('history')

// 探测历史相关状态
const historyData = ref([])
const historyLoading = ref(false)
const historyTotal = ref(0)
const historyPage = ref(1)
const historyPageSize = ref(20)
const historyIpFilter = ref('')
const historyStatusFilter = ref('')

// 操作日志相关状态
const logsData = ref([])
const logsLoading = ref(false)
const logsTotal = ref(0)
const logsPage = ref(1)
const logsPageSize = ref(20)
const logsIpFilter = ref('')
const logsStatusFilter = ref('')
// 操作日志筛选：操作类型和时间范围
const logsOperationType = ref('')
const logsStartTime = ref(null)
const logsEndTime = ref(null)

// IP 管理相关状态
const ipsData = ref([])
const ipsLoading = ref(false)

// CNAME 信息相关状态
const cnameLoading = ref(false)
const cnameInfo = reactive({
  targets: [],
  failed_count: 0,
  threshold: 0,
  threshold_type: 'count'
})

// ==================== 计算属性 ====================

/**
 * 格式化域名显示
 */
const taskDomainDisplay = computed(() => {
  if (!task.domain) return ''
  if (!task.sub_domain || task.sub_domain === '@') {
    return task.domain
  }
  return `${task.sub_domain}.${task.domain}`
})

// ==================== 辅助函数 ====================

/**
 * 操作类型中文标签映射
 * @param {string} type - 操作类型代码
 * @returns {string} 中文标签
 */
const operationTypeLabel = (type) => {
  const labels = {
    pause: '暂停',
    delete: '删除',
    resume: '恢复',
    add: '添加',
    switch: '切换'
  }
  return labels[type] || type
}

/**
 * 返回任务列表页面
 */
const goBack = () => {
  router.push('/tasks')
}

// ==================== API 调用 ====================

/**
 * 获取任务基本信息
 * 调用 GET /api/tasks/:id
 */
const fetchTask = async () => {
  taskLoading.value = true
  try {
    const response = await api.get(`/tasks/${taskId}`)
    Object.assign(task, response.data)
  } catch (error) {
    ElMessage.error('获取任务信息失败')
  } finally {
    taskLoading.value = false
  }
}

/**
 * 获取探测历史数据
 * 调用 GET /api/tasks/:id/history?ip=xxx&page=1&page_size=20
 */
const fetchHistory = async () => {
  historyLoading.value = true
  try {
    const params = {
      page: historyPage.value,
      page_size: historyPageSize.value
    }
    // 如果有 IP 筛选条件，添加到请求参数
    if (historyIpFilter.value.trim()) {
      params.ip = historyIpFilter.value.trim()
    }
    // 如果有状态筛选条件，添加到请求参数
    if (historyStatusFilter.value !== '' && historyStatusFilter.value !== null) {
      params.success = historyStatusFilter.value
    }
    const response = await api.get(`/tasks/${taskId}/history`, { params })
    historyData.value = response.data.data || []
    historyTotal.value = response.data.total || 0
  } catch (error) {
    ElMessage.error('获取探测历史失败')
  } finally {
    historyLoading.value = false
  }
}

/**
 * 获取操作日志数据
 * 调用 GET /api/tasks/:id/logs?page=1&page_size=20
 */
const fetchLogs = async () => {
  logsLoading.value = true
  try {
    const params = {
      page: logsPage.value,
      page_size: logsPageSize.value
    }
    // 如果有 IP 筛选条件，添加到请求参数
    if (logsIpFilter.value.trim()) {
      params.ip = logsIpFilter.value.trim()
    }
    // 如果有状态筛选条件，添加到请求参数
    if (logsStatusFilter.value !== '' && logsStatusFilter.value !== null) {
      params.success = logsStatusFilter.value
    }
    // 如果有操作类型筛选条件
    if (logsOperationType.value) {
      params.operation_type = logsOperationType.value
    }
    // 如果有开始时间筛选条件
    if (logsStartTime.value) {
      params.start_time = logsStartTime.value.toISOString()
    }
    // 如果有结束时间筛选条件
    if (logsEndTime.value) {
      params.end_time = logsEndTime.value.toISOString()
    }
    const response = await api.get(`/tasks/${taskId}/logs`, { params })
    logsData.value = response.data.data || []
    logsTotal.value = response.data.total || 0
  } catch (error) {
    ElMessage.error('获取操作日志失败')
  } finally {
    logsLoading.value = false
  }
}

/**
 * 获取任务关联的所有 IP 及其探测状态
 * 调用 GET /api/tasks/:id/ips
 */
const fetchIPs = async () => {
  ipsLoading.value = true
  try {
    const response = await api.get(`/tasks/${taskId}/ips`)
    ipsData.value = response.data || []
  } catch (error) {
    ElMessage.error('获取 IP 列表失败')
  } finally {
    ipsLoading.value = false
  }
}

/**
 * 排除某个 IP 的探测
 * @param {Object} row - IP 行数据
 */
const handleExcludeIP = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要排除 IP「${row.ip}」的探测吗？排除后该 IP 将不再被探测。`,
      '排除确认',
      { confirmButtonText: '确定排除', cancelButtonText: '取消', type: 'warning' }
    )
    await api.post(`/tasks/${taskId}/ips/exclude`, { ip: row.ip })
    ElMessage.success(`已排除 IP: ${row.ip}`)
    fetchIPs()
  } catch (err) {
    if (err !== 'cancel' && err?.toString() !== 'cancel') {
      ElMessage.error(err?.response?.data?.error || '排除 IP 失败')
    }
  }
}

/**
 * 恢复某个 IP 的探测
 * @param {Object} row - IP 行数据
 */
const handleIncludeIP = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要恢复 IP「${row.ip}」的探测吗？恢复后该 IP 将重新纳入探测。`,
      '恢复确认',
      { confirmButtonText: '确定恢复', cancelButtonText: '取消', type: 'info' }
    )
    await api.post(`/tasks/${taskId}/ips/include`, { ip: row.ip })
    ElMessage.success(`已恢复 IP: ${row.ip}`)
    fetchIPs()
  } catch (err) {
    if (err !== 'cancel' && err?.toString() !== 'cancel') {
      ElMessage.error(err?.response?.data?.error || '恢复 IP 失败')
    }
  }
}

/**
 * 获取CNAME信息
 * 从 GET /api/tasks/:id/cname 获取CNAME解析目标IP及健康状态
 */
const fetchCnameInfo = async () => {
  cnameLoading.value = true
  try {
    const response = await api.get(`/tasks/${taskId}/cname`)
    const data = response.data || {}
    cnameInfo.targets = data.targets || []
    cnameInfo.failed_count = data.failed_count || 0
    cnameInfo.threshold = data.threshold || 0
    cnameInfo.threshold_type = data.threshold_type || 'count'
  } catch (error) {
    ElMessage.error('获取CNAME信息失败')
  } finally {
    cnameLoading.value = false
  }
}

/**
 * 标签页切换时加载对应数据
 * @param {string} tabName - 标签页名称
 */
const handleTabChange = (tabName) => {
  if (tabName === 'history') {
    fetchHistory()
  } else if (tabName === 'logs') {
    fetchLogs()
  } else if (tabName === 'ips') {
    fetchIPs()
  } else if (tabName === 'cname') {
    fetchCnameInfo()
  }
}

// ==================== 生命周期 ====================

// 页面加载时获取任务信息和探测历史
onMounted(() => {
  fetchTask()
  fetchHistory()
})
</script>

<style scoped>
/* 任务详情页面容器 */
.task-detail-page {
  padding: 20px;
}

/* 页面头部：标题和操作按钮水平排列 */
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

/* 页面标题样式 */
.page-title {
  margin: 0;
  font-size: 20px;
  color: #303133;
  font-weight: 600;
}

/* 基本信息卡片 */
.info-card {
  margin-bottom: 20px;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

/* 切换状态卡片 */
.switch-card {
  margin-bottom: 20px;
}

/* 标签页卡片 */
.tabs-card {
  margin-bottom: 20px;
}

/* CNAME 阈值信息区域 */
.cname-threshold-info {
  margin-bottom: 16px;
}

/* 筛选栏 */
.filter-bar {
  margin-bottom: 16px;
}

/* 分页栏 */
.pagination-bar {
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
}
</style>
