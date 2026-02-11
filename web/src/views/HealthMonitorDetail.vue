<template>
  <!-- 健康监控任务详情页面 -->
  <div class="task-detail-page">
    <!-- 页面头部：域名信息和返回按钮 -->
    <div class="page-header">
      <h2 class="page-title">
        健康监控详情 - {{ taskDomainDisplay }}
      </h2>
      <el-button @click="goBack">返回列表</el-button>
    </div>

    <!-- 任务基本信息卡片 -->
    <el-card class="info-card" shadow="never" v-loading="taskLoading">
      <template #header>
        <span class="card-title">基本信息</span>
      </template>
      <el-descriptions :column="descriptionColumns" border>
        <el-descriptions-item label="域名">{{ task.domain }}</el-descriptions-item>
        <el-descriptions-item label="主机记录">{{ task.sub_domain }}</el-descriptions-item>
        <el-descriptions-item label="记录类型">{{ task.record_type }}</el-descriptions-item>
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
      </el-descriptions>
    </el-card>

    <!-- 标签页：基本信息 / 探测历史 / 监控目标 -->
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

          <!-- 延迟统计概览（使用CSS进度条可视化） -->
          <el-card v-if="historyStats.total > 0" class="stats-card" shadow="never">
            <div class="stats-row">
              <div class="stat-item">
                <span class="stat-label">总探测次数</span>
                <span class="stat-value">{{ historyStats.total }}</span>
              </div>
              <div class="stat-item">
                <span class="stat-label">成功次数</span>
                <span class="stat-value stat-success">{{ historyStats.successCount }}</span>
              </div>
              <div class="stat-item">
                <span class="stat-label">失败次数</span>
                <span class="stat-value stat-danger">{{ historyStats.failCount }}</span>
              </div>
              <div class="stat-item">
                <span class="stat-label">成功率</span>
                <span class="stat-value">{{ historyStats.successRate }}%</span>
              </div>
              <div class="stat-item">
                <span class="stat-label">平均延迟</span>
                <span class="stat-value">{{ historyStats.avgLatency }} ms</span>
              </div>
            </div>
            <!-- 成功率进度条 -->
            <div class="stats-progress">
              <span class="progress-label">成功率</span>
              <el-progress
                :percentage="historyStats.successRate"
                :color="successRateColor"
                :stroke-width="16"
                style="flex: 1"
              />
            </div>
          </el-card>

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

        <!-- 监控目标标签页 -->
        <el-tab-pane label="监控目标" name="targets">
          <el-table
            :data="task.targets || []"
            v-loading="taskLoading"
            border
            stripe
            style="width: 100%"
            empty-text="暂无监控目标"
          >
            <el-table-column prop="ip" label="IP 地址" min-width="150" />
            <el-table-column label="健康状态" min-width="120">
              <template #default="{ row }">
                <el-tag v-if="row.health_status === 'healthy'" type="success">健康</el-tag>
                <el-tag v-else-if="row.health_status === 'unhealthy'" type="danger">不健康</el-tag>
                <el-tag v-else type="info">未知</el-tag>
              </template>
            </el-table-column>
            <el-table-column label="连续失败" min-width="100">
              <template #default="{ row }">
                <span :class="{ 'text-danger': row.consecutive_fails > 0 }">
                  {{ row.consecutive_fails }}
                </span>
              </template>
            </el-table-column>
            <el-table-column label="连续成功" min-width="100">
              <template #default="{ row }">
                <span :class="{ 'text-success': row.consecutive_successes > 0 }">
                  {{ row.consecutive_successes }}
                </span>
              </template>
            </el-table-column>
            <el-table-column label="平均延迟" min-width="100">
              <template #default="{ row }">
                {{ row.avg_latency_ms ? row.avg_latency_ms + ' ms' : '-' }}
              </template>
            </el-table-column>
            <el-table-column label="最后探测时间" min-width="180">
              <template #default="{ row }">
                {{ row.last_probe_at || '-' }}
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

      </el-tabs>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted, computed, onBeforeUnmount } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import api from '../api'

// ==================== 路由 ====================

const route = useRoute()
const router = useRouter()

// 任务 ID，从路由参数获取
const taskId = route.params.id

// ==================== 响应式列数 ====================

// el-descriptions 响应式列数
const descriptionColumns = ref(3)

/**
 * 根据窗口宽度更新 el-descriptions 列数
 */
const updateColumns = () => {
  const width = window.innerWidth
  if (width < 576) {
    descriptionColumns.value = 1
  } else if (width < 992) {
    descriptionColumns.value = 2
  } else {
    descriptionColumns.value = 3
  }
}

// 监听窗口大小变化
window.addEventListener('resize', updateColumns)
updateColumns()

onBeforeUnmount(() => {
  window.removeEventListener('resize', updateColumns)
})

// ==================== 状态定义 ====================

// 任务基本信息（包含 targets）
const task = reactive({
  id: null,
  domain: '',
  sub_domain: '',
  record_type: '',
  probe_protocol: '',
  probe_port: 0,
  probe_interval_sec: 0,
  timeout_ms: 0,
  fail_threshold: 0,
  recover_threshold: 0,
  enabled: false,
  targets: []
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

/**
 * 探测历史统计数据
 * 基于当前页面数据计算成功率、平均延迟等
 */
const historyStats = computed(() => {
  const data = historyData.value
  if (!data || data.length === 0) {
    return { total: 0, successCount: 0, failCount: 0, successRate: 0, avgLatency: 0 }
  }
  const total = data.length
  const successCount = data.filter(r => r.success).length
  const failCount = total - successCount
  const successRate = total > 0 ? Math.round((successCount / total) * 100) : 0
  // 计算成功探测的平均延迟
  const successItems = data.filter(r => r.success && r.latency_ms != null)
  const avgLatency = successItems.length > 0
    ? Math.round(successItems.reduce((sum, r) => sum + r.latency_ms, 0) / successItems.length)
    : 0
  return { total, successCount, failCount, successRate, avgLatency }
})

/**
 * 成功率进度条颜色
 */
const successRateColor = computed(() => {
  const rate = historyStats.value.successRate
  if (rate >= 90) return '#67c23a'
  if (rate >= 70) return '#e6a23c'
  return '#f56c6c'
})

// ==================== 辅助函数 ====================

/**
 * 返回健康监控列表页面
 */
const goBack = () => {
  router.push('/health-monitors')
}

// ==================== API 调用 ====================

/**
 * 获取任务详情（包含监控目标列表）
 * 调用 GET /api/health-monitors/:id
 */
const fetchTask = async () => {
  taskLoading.value = true
  try {
    const response = await api.get(`/health-monitors/${taskId}`)
    const data = response.data.data || response.data
    Object.assign(task, data)
  } catch (error) {
    ElMessage.error('获取任务信息失败')
  } finally {
    taskLoading.value = false
  }
}

/**
 * 获取探测历史数据
 * 调用 GET /api/health-monitors/:id/results
 */
const fetchHistory = async () => {
  historyLoading.value = true
  try {
    const params = {
      page: historyPage.value,
      page_size: historyPageSize.value
    }
    // IP 筛选
    if (historyIpFilter.value.trim()) {
      params.ip = historyIpFilter.value.trim()
    }
    // 状态筛选
    if (historyStatusFilter.value !== '' && historyStatusFilter.value !== null) {
      params.success = historyStatusFilter.value
    }
    const response = await api.get(`/health-monitors/${taskId}/results`, { params })
    const resData = response.data.data || response.data
    historyData.value = resData.items || []
    historyTotal.value = resData.total || 0
  } catch (error) {
    ElMessage.error('获取探测历史失败')
  } finally {
    historyLoading.value = false
  }
}

/**
 * 标签页切换时加载对应数据
 * @param {string} tabName - 标签页名称
 */
const handleTabChange = (tabName) => {
  if (tabName === 'history') {
    fetchHistory()
  } else if (tabName === 'targets') {
    // 监控目标数据已在任务详情中返回，重新获取最新数据
    fetchTask()
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

/* 标签页卡片 */
.tabs-card {
  margin-bottom: 20px;
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

/* 统计概览卡片 */
.stats-card {
  margin-bottom: 16px;
}

.stats-row {
  display: flex;
  justify-content: space-around;
  margin-bottom: 12px;
}

.stat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}

.stat-label {
  font-size: 12px;
  color: #909399;
}

.stat-value {
  font-size: 20px;
  font-weight: 600;
  color: #303133;
}

.stat-success {
  color: #67c23a;
}

.stat-danger {
  color: #f56c6c;
}

/* 成功率进度条 */
.stats-progress {
  display: flex;
  align-items: center;
  gap: 12px;
}

.progress-label {
  font-size: 13px;
  color: #606266;
  white-space: nowrap;
}

/* 文字颜色辅助类 */
.text-danger {
  color: #f56c6c;
  font-weight: 600;
}

.text-success {
  color: #67c23a;
  font-weight: 600;
}

/* ==================== 响应式适配 ==================== */

@media screen and (max-width: 768px) {
  .task-detail-page {
    padding: 12px;
  }

  /* 页面头部在小屏幕上堆叠显示 */
  .page-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 10px;
  }

  .page-title {
    font-size: 16px;
  }

  /* 统计概览在小屏幕上换行显示 */
  .stats-row {
    flex-wrap: wrap;
    gap: 12px;
  }

  .stat-item {
    min-width: 80px;
  }

  .stat-value {
    font-size: 16px;
  }

  /* 筛选栏在小屏幕上堆叠 */
  .filter-bar {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .filter-bar .el-input,
  .filter-bar .el-select {
    width: 100% !important;
    margin-right: 0 !important;
  }

  /* 分页组件简化 */
  .pagination-bar :deep(.el-pagination) {
    flex-wrap: wrap;
    justify-content: center;
    gap: 4px;
  }
}
</style>
