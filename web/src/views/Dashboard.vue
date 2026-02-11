<template>
  <!-- 系统总览页面 -->
  <div class="dashboard-page">
    <h2 class="page-title">系统总览</h2>

    <!-- 系统时间与运行时间 -->
    <el-row :gutter="16" class="summary-row">
      <el-col :span="12">
        <el-card shadow="hover" class="time-card">
          <div class="time-info">
            <el-icon :size="28" color="#409eff"><Clock /></el-icon>
            <div class="time-detail">
              <div class="time-label">当前系统时间</div>
              <div class="time-value">{{ currentTime }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card shadow="hover" class="time-card">
          <div class="time-info">
            <el-icon :size="28" color="#67c23a"><Timer /></el-icon>
            <div class="time-detail">
              <div class="time-label">程序已运行时间</div>
              <div class="time-value">{{ uptimeText }}</div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 任务统计卡片 -->
    <el-row :gutter="16" class="summary-row">
      <el-col :span="6">
        <el-card shadow="hover" class="summary-card">
          <div class="summary-number">{{ stats.total_tasks }}</div>
          <div class="summary-label">探测任务总数</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="summary-card">
          <div class="summary-number running">{{ stats.running_tasks }}</div>
          <div class="summary-label">运行中</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="summary-card">
          <div class="summary-number stopped">{{ stats.stopped_tasks }}</div>
          <div class="summary-label">已停止</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover" class="summary-card">
          <div class="summary-number probe-total">{{ stats.total_probes }}</div>
          <div class="summary-label">总探测次数</div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 探测统计 + 健康状态分布 -->
    <el-row :gutter="16" class="summary-row equal-height-row">
      <el-col :span="12">
        <el-card shadow="never" class="equal-height-card">
          <template #header>
            <span class="card-title">探测统计</span>
          </template>
          <div class="card-body-inner">
            <el-row :gutter="16">
              <el-col :span="12">
                <div class="stat-item success-stat">
                  <div class="stat-number">{{ stats.success_probes }}</div>
                  <div class="stat-label">探测成功</div>
                </div>
              </el-col>
              <el-col :span="12">
                <div class="stat-item fail-stat">
                  <div class="stat-number">{{ stats.failed_probes }}</div>
                  <div class="stat-label">探测失败</div>
                </div>
              </el-col>
            </el-row>
            <div class="probe-rate">
              <el-progress
                v-if="stats.total_probes > 0"
                :percentage="successRate"
                :color="successRate >= 90 ? '#67c23a' : successRate >= 60 ? '#e6a23c' : '#f56c6c'"
              />
              <div class="rate-label">
                {{ stats.total_probes > 0 ? '探测成功率 ' + successRate + '%' : '暂无探测数据' }}
              </div>
            </div>
          </div>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card shadow="never" class="equal-height-card">
          <template #header>
            <span class="card-title">任务健康状态分布</span>
          </template>
          <div class="card-body-inner">
            <div class="health-grid">
              <div class="health-item">
                <el-tag type="success" size="large" effect="dark">正常</el-tag>
                <span class="health-count">{{ stats.normal_tasks }}</span>
              </div>
              <div class="health-item">
                <el-tag type="warning" size="large" effect="dark">异常</el-tag>
                <span class="health-count">{{ stats.abnormal_tasks }}</span>
              </div>
              <div class="health-item">
                <el-tag type="danger" size="large" effect="dark">失败</el-tag>
                <span class="health-count">{{ stats.failed_tasks }}</span>
              </div>
              <div class="health-item">
                <el-tag type="info" size="large" effect="dark">未知</el-tag>
                <span class="health-count">{{ stats.unknown_tasks }}</span>
              </div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>

    <!-- 最近系统日志 -->
    <el-card shadow="never" class="events-card">
      <template #header>
        <div class="events-header">
          <span class="card-title">最近系统日志</span>
          <el-button type="primary" size="small" text @click="goToSystemLogs">查看全部</el-button>
        </div>
      </template>
      <el-table
        :data="stats.recent_events"
        border
        stripe
        style="width: 100%"
        empty-text="暂无系统日志"
        size="small"
      >
        <el-table-column prop="timestamp" label="时间" width="170" />
        <el-table-column label="来源" width="80" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.source === 'operation'" type="primary" size="small">操作</el-tag>
            <el-tag v-else type="warning" size="small">通知</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="task_name" label="任务" min-width="140" show-overflow-tooltip />
        <el-table-column label="类型" width="100">
          <template #default="{ row }">
            {{ formatLogType(row.source, row.type) }}
          </template>
        </el-table-column>
        <el-table-column label="结果" width="80" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.success" type="success" size="small">成功</el-tag>
            <el-tag v-else type="danger" size="small">失败</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="detail" label="详情" min-width="180" show-overflow-tooltip />
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Clock, Timer } from '@element-plus/icons-vue'
import api from '../api'

// ==================== 路由 ====================

const router = useRouter()

// ==================== 状态定义 ====================

// 当前系统时间（每秒更新）
const currentTime = ref('')
// 程序启动时间（从后端获取）
const serverStartTime = ref(null)
// 运行时长文本
const uptimeText = ref('加载中...')
// 定时器引用
let timeTimer = null

// 系统总览统计数据
const stats = ref({
  total_tasks: 0,
  running_tasks: 0,
  stopped_tasks: 0,
  total_probes: 0,
  success_probes: 0,
  failed_probes: 0,
  normal_tasks: 0,
  abnormal_tasks: 0,
  failed_tasks: 0,
  unknown_tasks: 0,
  recent_events: []
})

// ==================== 计算属性 ====================

/**
 * 探测成功率
 */
const successRate = computed(() => {
  if (stats.value.total_probes === 0) return 0
  return Math.round((stats.value.success_probes / stats.value.total_probes) * 10000) / 100
})

// ==================== 辅助函数 ====================

/**
 * 日志类型中文标签映射
 */
const formatLogType = (source, type) => {
  if (source === 'operation') {
    const labels = { pause: '暂停', delete: '删除', resume: '恢复', add: '添加' }
    return labels[type] || type
  }
  const labels = { failover: '故障转移', recovery: '恢复', consecutive_fail: '连续失败' }
  return labels[type] || type
}

/**
 * 跳转到系统日志页面
 */
const goToSystemLogs = () => {
  router.push('/system-logs')
}

// ==================== API 调用 ====================

/**
 * 获取系统总览统计数据
 * 调用 GET /api/dashboard/stats
 */
const fetchStats = async () => {
  try {
    const response = await api.get('/dashboard/stats')
    stats.value = response.data
  } catch (error) {
    ElMessage.error('获取系统统计数据失败')
  }
}

/**
 * 获取服务器启动时间
 * 调用 GET /api/system-info
 */
const fetchSystemInfo = async () => {
  try {
    const res = await api.get('/system-info')
    if (res.data.start_time) {
      serverStartTime.value = new Date(res.data.start_time)
    }
  } catch {
    // 获取失败不影响其他功能
  }
}

/**
 * 更新当前时间和运行时长
 */
const updateTime = () => {
  const now = new Date()
  currentTime.value = now.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false
  })

  // 计算运行时长
  if (serverStartTime.value) {
    const diff = now - serverStartTime.value
    const days = Math.floor(diff / (1000 * 60 * 60 * 24))
    const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60))
    const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60))
    const seconds = Math.floor((diff % (1000 * 60)) / 1000)

    const parts = []
    if (days > 0) parts.push(`${days} 天`)
    if (hours > 0) parts.push(`${hours} 小时`)
    if (minutes > 0) parts.push(`${minutes} 分钟`)
    parts.push(`${seconds} 秒`)
    uptimeText.value = parts.join(' ')
  }
}

// ==================== 生命周期 ====================

onMounted(async () => {
  fetchStats()
  await fetchSystemInfo()
  updateTime()
  // 每秒更新时间
  timeTimer = setInterval(updateTime, 1000)
})

onUnmounted(() => {
  if (timeTimer) {
    clearInterval(timeTimer)
  }
})
</script>

<style scoped>
.dashboard-page {
  padding: 20px;
}

.page-title {
  margin: 0 0 20px 0;
  font-size: 20px;
  color: #303133;
  font-weight: 600;
}

.summary-row {
  margin-bottom: 16px;
}

/* 时间信息卡片 */
.time-card :deep(.el-card__body) {
  padding: 16px 20px;
}

.time-info {
  display: flex;
  align-items: center;
  gap: 14px;
}

.time-label {
  font-size: 13px;
  color: #909399;
  margin-bottom: 4px;
}

.time-value {
  font-size: 18px;
  font-weight: 600;
  color: #303133;
  font-variant-numeric: tabular-nums;
}

.summary-card {
  text-align: center;
}

.summary-number {
  font-size: 32px;
  font-weight: 700;
  color: #409eff;
  margin-bottom: 6px;
  padding-top: 8px;
}

.summary-number.running {
  color: #67c23a;
}

.summary-number.stopped {
  color: #909399;
}

.summary-number.probe-total {
  color: #e6a23c;
}

.summary-label {
  font-size: 13px;
  color: #606266;
  padding-bottom: 8px;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.stat-item {
  text-align: center;
  padding: 12px 0;
}

.stat-number {
  font-size: 28px;
  font-weight: 700;
  margin-bottom: 4px;
}

.success-stat .stat-number {
  color: #67c23a;
}

.fail-stat .stat-number {
  color: #f56c6c;
}

.stat-label {
  font-size: 13px;
  color: #606266;
}

.probe-rate {
  margin-top: 16px;
  padding-top: 12px;
  border-top: 1px solid #ebeef5;
}

.rate-label {
  text-align: center;
  font-size: 13px;
  color: #606266;
  margin-top: 6px;
}

/* 第二行等高布局 */
.equal-height-row {
  display: flex;
  flex-wrap: wrap;
}

.equal-height-row > .el-col {
  display: flex;
}

.equal-height-card {
  width: 100%;
  display: flex;
  flex-direction: column;
}

.equal-height-card :deep(.el-card__body) {
  flex: 1;
  display: flex;
  flex-direction: column;
}

.card-body-inner {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.health-grid {
  display: flex;
  justify-content: space-around;
  padding: 16px 0;
}

.health-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
}

.health-count {
  font-size: 24px;
  font-weight: 700;
  color: #303133;
}

.events-card {
  margin-bottom: 20px;
}

.events-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
