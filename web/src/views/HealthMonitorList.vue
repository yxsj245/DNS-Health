<template>
  <!-- 健康监控任务列表页面 -->
  <div class="task-list-page">
    <!-- 页面头部：标题和创建按钮 -->
    <div class="page-header">
      <h2 class="page-title">健康监控</h2>
      <el-button type="primary" @click="handleCreate">
        创建任务
      </el-button>
    </div>

    <!-- 任务列表表格 -->
    <el-table
      :data="monitors"
      v-loading="tableLoading"
      border
      stripe
      style="width: 100%"
      empty-text="暂无健康监控任务"
    >
      <!-- 域名列：合并 sub_domain 和 domain 显示 -->
      <el-table-column label="域名" min-width="180">
        <template #default="{ row }">
          {{ formatDomain(row.sub_domain, row.domain) }}
        </template>
      </el-table-column>

      <!-- 记录类型列 -->
      <el-table-column prop="record_type" label="记录类型" min-width="100" />

      <!-- 探测协议列 -->
      <el-table-column prop="probe_protocol" label="探测协议" min-width="100" />

      <!-- 探测周期列 -->
      <el-table-column label="探测周期" min-width="100">
        <template #default="{ row }">
          {{ row.probe_interval_sec }} 秒
        </template>
      </el-table-column>

      <!-- 超时时间列 -->
      <el-table-column label="超时时间" min-width="100">
        <template #default="{ row }">
          {{ row.timeout_ms }} 毫秒
        </template>
      </el-table-column>

      <!-- 状态列：运行中/已停止 -->
      <el-table-column label="状态" min-width="100">
        <template #default="{ row }">
          <el-tag v-if="row.enabled" type="success">运行中</el-tag>
          <el-tag v-else type="info">已停止</el-tag>
        </template>
      </el-table-column>

      <!-- 健康状态列：显示健康/不健康/总数统计 -->
      <el-table-column label="健康状态" min-width="150">
        <template #default="{ row }">
          <span class="health-stats">
            <el-tag type="success" size="small">{{ row.healthy_count || 0 }}</el-tag>
            <span class="health-separator">/</span>
            <el-tag type="danger" size="small">{{ row.unhealthy_count || 0 }}</el-tag>
            <span class="health-separator">/</span>
            <el-tag type="info" size="small">{{ row.target_count || 0 }}</el-tag>
          </span>
          <div class="health-label">健康 / 不健康 / 总数</div>
        </template>
      </el-table-column>

      <!-- 操作列 -->
      <el-table-column label="操作" width="340" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" size="small" @click="handleDetail(row)">
            查看详情
          </el-button>
          <el-button type="warning" size="small" @click="handleEdit(row)">
            编辑
          </el-button>
          <!-- 暂停/恢复按钮 -->
          <el-button
            v-if="row.enabled"
            type="info"
            size="small"
            @click="handlePause(row)"
          >
            暂停
          </el-button>
          <el-button
            v-else
            type="success"
            size="small"
            @click="handleResume(row)"
          >
            恢复
          </el-button>
          <el-button type="danger" size="small" @click="handleDelete(row)">
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage, ElMessageBox } from 'element-plus'
import api from '../api'

// ==================== 状态定义 ====================

const router = useRouter()

// 监控任务列表数据
const monitors = ref([])

// 表格加载状态
const tableLoading = ref(false)

// ==================== 辅助函数 ====================

/**
 * 格式化域名显示
 * 将 sub_domain 和 domain 合并为完整域名
 * @param {string} subDomain - 主机记录
 * @param {string} domain - 主域名
 * @returns {string} 格式化后的完整域名
 */
const formatDomain = (subDomain, domain) => {
  if (!subDomain || subDomain === '@') {
    return domain
  }
  return `${subDomain}.${domain}`
}

// ==================== API 调用 ====================

/**
 * 获取健康监控任务列表
 * 调用 GET /api/health-monitors 接口
 */
const fetchMonitors = async () => {
  tableLoading.value = true
  try {
    const response = await api.get('/health-monitors')
    monitors.value = response.data.data || []
  } catch (error) {
    ElMessage.error('获取健康监控任务列表失败')
  } finally {
    tableLoading.value = false
  }
}

/**
 * 跳转到创建任务页面
 */
const handleCreate = () => {
  router.push('/health-monitors/new')
}

/**
 * 跳转到任务详情页面
 * @param {Object} row - 任务行数据
 */
const handleDetail = (row) => {
  router.push(`/health-monitors/${row.id}`)
}

/**
 * 跳转到编辑任务页面
 * @param {Object} row - 任务行数据
 */
const handleEdit = (row) => {
  router.push(`/health-monitors/${row.id}/edit`)
}

/**
 * 暂停监控任务
 * @param {Object} row - 要暂停的任务行数据
 */
const handlePause = async (row) => {
  const domainDisplay = formatDomain(row.sub_domain, row.domain)
  try {
    await ElMessageBox.confirm(
      `确定要暂停健康监控任务「${domainDisplay}」吗？暂停后将停止该任务的探测。`,
      '暂停确认',
      {
        confirmButtonText: '确定暂停',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    tableLoading.value = true
    try {
      await api.post(`/health-monitors/${row.id}/pause`)
      ElMessage.success('任务已暂停')
      await fetchMonitors()
    } catch (error) {
      ElMessage.error('暂停任务失败')
      tableLoading.value = false
    }
  } catch {
    // 用户取消操作
  }
}

/**
 * 恢复监控任务
 * @param {Object} row - 要恢复的任务行数据
 */
const handleResume = async (row) => {
  const domainDisplay = formatDomain(row.sub_domain, row.domain)
  try {
    await ElMessageBox.confirm(
      `确定要恢复健康监控任务「${domainDisplay}」吗？恢复后将重新开始探测。`,
      '恢复确认',
      {
        confirmButtonText: '确定恢复',
        cancelButtonText: '取消',
        type: 'info'
      }
    )

    tableLoading.value = true
    try {
      await api.post(`/health-monitors/${row.id}/resume`)
      ElMessage.success('任务已恢复')
      await fetchMonitors()
    } catch (error) {
      ElMessage.error('恢复任务失败')
      tableLoading.value = false
    }
  } catch {
    // 用户取消操作
  }
}

/**
 * 删除监控任务
 * @param {Object} row - 要删除的任务行数据
 */
const handleDelete = async (row) => {
  const domainDisplay = formatDomain(row.sub_domain, row.domain)
  try {
    await ElMessageBox.confirm(
      `确定要删除健康监控任务「${domainDisplay}」吗？删除后将停止该任务的探测，且不可恢复。`,
      '删除确认',
      {
        confirmButtonText: '确定删除',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    tableLoading.value = true
    try {
      await api.delete(`/health-monitors/${row.id}`)
      ElMessage.success('任务已删除')
      await fetchMonitors()
    } catch (error) {
      ElMessage.error('删除任务失败')
      tableLoading.value = false
    }
  } catch {
    // 用户取消操作
  }
}

// ==================== 生命周期 ====================

// 页面加载时获取任务列表
onMounted(() => {
  fetchMonitors()
})
</script>

<style scoped>
/* 任务列表页面容器 */
.task-list-page {
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

/* 健康状态统计样式 */
.health-stats {
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.health-separator {
  color: #909399;
  font-size: 12px;
  margin: 0 2px;
}

.health-label {
  font-size: 11px;
  color: #909399;
  margin-top: 2px;
}

/* ==================== 响应式适配 ==================== */

@media screen and (max-width: 768px) {
  .task-list-page {
    padding: 12px;
  }

  .page-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 10px;
  }

  .page-title {
    font-size: 16px;
  }
}
</style>
