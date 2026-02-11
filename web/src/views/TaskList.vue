<template>
  <!-- 探测任务列表页面 -->
  <div class="task-list-page">
    <!-- 页面头部：标题和创建按钮 -->
    <div class="page-header">
      <h2 class="page-title">探测任务列表</h2>
      <el-button type="primary" @click="handleCreate">
        创建任务
      </el-button>
    </div>

    <!-- 任务列表表格 -->
    <el-table
      :data="tasks"
      v-loading="tableLoading"
      border
      stripe
      style="width: 100%"
      empty-text="暂无探测任务"
    >
      <!-- 域名列：合并 domain 和 sub_domain 显示 -->
      <el-table-column label="域名" min-width="180">
        <template #default="{ row }">
          {{ formatDomain(row.sub_domain, row.domain) }}
        </template>
      </el-table-column>

      <!-- 任务类型列 -->
      <el-table-column label="任务类型" min-width="110">
        <template #default="{ row }">
          {{ row.task_type === 'switch' ? '切换解析' : (row.task_type === 'cdn_switch' ? 'CDN 故障转移' : '暂停/删除') }}
        </template>
      </el-table-column>

      <!-- 记录类型列 -->
      <el-table-column label="记录类型" min-width="100">
        <template #default="{ row }">
          {{ row.record_type === 'CNAME' ? 'CNAME' : 'A/AAAA' }}
        </template>
      </el-table-column>

      <!-- 切换状态列 -->
      <el-table-column label="切换状态" min-width="100">
        <template #default="{ row }">
          <el-tag v-if="row.is_switched" type="danger">已切换</el-tag>
          <span v-else>-</span>
        </template>
      </el-table-column>

      <!-- 解析池列 -->
      <el-table-column label="解析池" min-width="120">
        <template #default="{ row }">
          {{ row.pool_name || '-' }}
        </template>
      </el-table-column>

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

      <!-- 健康状态列 -->
      <el-table-column label="健康状态" min-width="110">
        <template #default="{ row }">
          <el-tag v-if="healthMap[row.id] === 'normal'" type="success">正常</el-tag>
          <el-tag v-else-if="healthMap[row.id] === 'abnormal'" type="warning">异常</el-tag>
          <el-tag v-else-if="healthMap[row.id] === 'failed'" type="danger">失败</el-tag>
          <el-tag v-else type="info">未知</el-tag>
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

// 任务列表数据
const tasks = ref([])

// 表格加载状态
const tableLoading = ref(false)

// 任务健康状态映射 { taskId: 'normal' | 'abnormal' | 'failed' | 'unknown' }
const healthMap = ref({})

// ==================== 辅助函数 ====================

/**
 * 格式化域名显示
 * 将 sub_domain 和 domain 合并为完整域名
 * 例如：sub_domain="www", domain="example.com" → "www.example.com"
 * 例如：sub_domain="@", domain="example.com" → "example.com"
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
 * 获取探测任务列表
 * 调用 GET /api/tasks 接口
 */
const fetchTasks = async () => {
  tableLoading.value = true
  try {
    const response = await api.get('/tasks')
    tasks.value = response.data
    // 获取任务列表后同时获取健康状态
    fetchHealthStatus()
  } catch (error) {
    ElMessage.error('获取任务列表失败')
  } finally {
    tableLoading.value = false
  }
}

/**
 * 获取所有任务的健康状态
 * 调用 GET /api/tasks/health 接口
 */
const fetchHealthStatus = async () => {
  try {
    const response = await api.get('/tasks/health')
    healthMap.value = response.data || {}
  } catch (error) {
    // 健康状态获取失败不影响主流程
  }
}

/**
 * 跳转到创建任务页面
 */
const handleCreate = () => {
  router.push('/tasks/new')
}

/**
 * 跳转到任务详情页面
 * @param {Object} row - 任务行数据
 */
const handleDetail = (row) => {
  router.push(`/tasks/${row.id}`)
}

/**
 * 跳转到编辑任务页面
 * @param {Object} row - 任务行数据
 */
const handleEdit = (row) => {
  router.push(`/tasks/${row.id}/edit`)
}

/**
 * 暂停探测任务
 * 弹出确认对话框，确认后调用 POST /api/tasks/:id/pause 接口
 * @param {Object} row - 要暂停的任务行数据
 */
const handlePause = async (row) => {
  const domainDisplay = formatDomain(row.sub_domain, row.domain)
  try {
    await ElMessageBox.confirm(
      `确定要暂停探测任务「${domainDisplay}」吗？暂停后将停止该任务的探测。`,
      '暂停确认',
      {
        confirmButtonText: '确定暂停',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    // 用户确认暂停
    tableLoading.value = true
    try {
      await api.post(`/tasks/${row.id}/pause`)
      ElMessage.success('任务已暂停')
      // 刷新任务列表
      await fetchTasks()
    } catch (error) {
      ElMessage.error('暂停任务失败')
      tableLoading.value = false
    }
  } catch {
    // 用户取消暂停，不做任何操作
  }
}

/**
 * 恢复探测任务
 * 弹出确认对话框，确认后调用 POST /api/tasks/:id/resume 接口
 * @param {Object} row - 要恢复的任务行数据
 */
const handleResume = async (row) => {
  const domainDisplay = formatDomain(row.sub_domain, row.domain)
  try {
    await ElMessageBox.confirm(
      `确定要恢复探测任务「${domainDisplay}」吗？恢复后将重新开始探测。`,
      '恢复确认',
      {
        confirmButtonText: '确定恢复',
        cancelButtonText: '取消',
        type: 'info'
      }
    )

    // 用户确认恢复
    tableLoading.value = true
    try {
      await api.post(`/tasks/${row.id}/resume`)
      ElMessage.success('任务已恢复')
      // 刷新任务列表
      await fetchTasks()
    } catch (error) {
      ElMessage.error('恢复任务失败')
      tableLoading.value = false
    }
  } catch {
    // 用户取消恢复，不做任何操作
  }
}

/**
 * 删除探测任务
 * 弹出确认对话框，确认后调用 DELETE /api/tasks/:id 接口
 * @param {Object} row - 要删除的任务行数据
 */
const handleDelete = async (row) => {
  const domainDisplay = formatDomain(row.sub_domain, row.domain)
  try {
    await ElMessageBox.confirm(
      `确定要删除探测任务「${domainDisplay}」吗？删除后将停止该任务的探测，且不可恢复。`,
      '删除确认',
      {
        confirmButtonText: '确定删除',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    // 用户确认删除
    tableLoading.value = true
    try {
      await api.delete(`/tasks/${row.id}`)
      ElMessage.success('任务已删除')
      // 刷新任务列表
      await fetchTasks()
    } catch (error) {
      ElMessage.error('删除任务失败')
      tableLoading.value = false
    }
  } catch {
    // 用户取消删除，不做任何操作
  }
}

// ==================== 生命周期 ====================

// 页面加载时获取任务列表
onMounted(() => {
  fetchTasks()
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
</style>
