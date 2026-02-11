<template>
  <!-- 系统日志页面：筛选 + 统一日志列表 + 分页 -->
  <div class="system-log-page">
    <!-- 页面头部 -->
    <div class="page-header">
      <h2 class="page-title">系统日志</h2>
    </div>

    <!-- 筛选区域 -->
    <el-card class="filter-card" shadow="never">
      <el-form :inline="true" class="filter-form">
        <!-- 日志来源选择器 -->
        <el-form-item label="来源">
          <el-select
            v-model="filters.source"
            placeholder="全部来源"
            clearable
            style="width: 160px"
            @change="handleSourceChange"
          >
            <el-option label="操作日志" value="operation" />
            <el-option label="通知记录" value="notification" />
          </el-select>
        </el-form-item>

        <!-- 类型选择器（根据来源动态变化） -->
        <el-form-item label="类型">
          <el-select
            v-model="filters.type"
            placeholder="全部类型"
            clearable
            style="width: 160px"
          >
            <template v-if="filters.source === 'operation'">
              <el-option label="暂停" value="pause" />
              <el-option label="删除" value="delete" />
              <el-option label="恢复" value="resume" />
              <el-option label="添加" value="add" />
            </template>
            <template v-else-if="filters.source === 'notification'">
              <el-option label="故障转移" value="failover" />
              <el-option label="恢复" value="recovery" />
              <el-option label="连续失败" value="consecutive_fail" />
            </template>
            <template v-else>
              <el-option label="暂停" value="pause" />
              <el-option label="删除" value="delete" />
              <el-option label="恢复" value="resume" />
              <el-option label="添加" value="add" />
              <el-option label="故障转移" value="failover" />
              <el-option label="恢复通知" value="recovery" />
              <el-option label="连续失败" value="consecutive_fail" />
            </template>
          </el-select>
        </el-form-item>

        <!-- 状态选择器 -->
        <el-form-item label="状态">
          <el-select
            v-model="filters.success"
            placeholder="全部状态"
            clearable
            style="width: 120px"
          >
            <el-option label="成功" value="true" />
            <el-option label="失败" value="false" />
          </el-select>
        </el-form-item>

        <!-- 查询按钮 -->
        <el-form-item>
          <el-button type="primary" @click="handleSearch">查询</el-button>
          <el-button @click="handleReset">重置</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <!-- 日志列表 -->
    <el-card class="log-card" shadow="never">
      <el-table
        :data="logList"
        v-loading="tableLoading"
        border
        stripe
        style="width: 100%"
        empty-text="暂无系统日志"
      >
        <!-- 时间列 -->
        <el-table-column prop="timestamp" label="时间" width="170" />

        <!-- 来源列 -->
        <el-table-column label="来源" width="100" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.source === 'operation'" type="primary" size="small">操作</el-tag>
            <el-tag v-else type="warning" size="small">通知</el-tag>
          </template>
        </el-table-column>

        <!-- 任务名称列 -->
        <el-table-column prop="task_name" label="任务" min-width="160" show-overflow-tooltip />

        <!-- 类型列 -->
        <el-table-column label="类型" width="120">
          <template #default="{ row }">
            {{ formatType(row.source, row.type) }}
          </template>
        </el-table-column>

        <!-- 状态列 -->
        <el-table-column label="状态" width="80" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.success" type="success" size="small">成功</el-tag>
            <el-tag v-else type="danger" size="small">失败</el-tag>
          </template>
        </el-table-column>

        <!-- 详情列 -->
        <el-table-column prop="detail" label="详情" min-width="220" show-overflow-tooltip />

        <!-- 附加信息列 -->
        <el-table-column label="附加信息" min-width="130" show-overflow-tooltip>
          <template #default="{ row }">
            <template v-if="row.source === 'operation'">
              {{ row.extra || '-' }}
            </template>
            <template v-else>
              {{ formatChannelType(row.extra) }}
            </template>
          </template>
        </el-table-column>

        <!-- 错误信息列 -->
        <el-table-column prop="error_msg" label="错误信息" min-width="180" show-overflow-tooltip>
          <template #default="{ row }">
            {{ row.error_msg || '-' }}
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页组件 -->
      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="pagination.page"
          v-model:page-size="pagination.pageSize"
          :total="pagination.total"
          :page-sizes="[10, 20, 50, 100]"
          layout="total, sizes, prev, pager, next, jumper"
          @size-change="handleSizeChange"
          @current-change="handlePageChange"
        />
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref, reactive, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api'

// ==================== 状态定义 ====================

// 表格加载状态
const tableLoading = ref(false)

// 日志列表
const logList = ref([])

// 筛选条件
const filters = reactive({
  source: '',
  type: '',
  success: ''
})

// 分页参数
const pagination = reactive({
  page: 1,
  pageSize: 20,
  total: 0
})

// ==================== 辅助函数 ====================

/**
 * 格式化类型显示
 * @param {string} source - 日志来源
 * @param {string} type - 类型标识
 * @returns {string} 中文类型名称
 */
const formatType = (source, type) => {
  if (source === 'operation') {
    const map = { pause: '暂停', delete: '删除', resume: '恢复', add: '添加' }
    return map[type] || type
  }
  const map = { failover: '故障转移', recovery: '恢复', consecutive_fail: '连续失败' }
  return map[type] || type
}

/**
 * 格式化渠道类型
 * @param {string} channelType - 渠道类型标识
 * @returns {string} 中文渠道名称
 */
const formatChannelType = (channelType) => {
  const map = { email: '邮件' }
  return map[channelType] || channelType || '-'
}

// ==================== API 调用 ====================

/**
 * 获取系统日志列表
 * 调用 GET /api/system-logs
 */
const fetchLogs = async () => {
  tableLoading.value = true
  try {
    const params = {
      page: pagination.page,
      page_size: pagination.pageSize
    }
    if (filters.source) params.source = filters.source
    if (filters.type) params.type = filters.type
    if (filters.success) params.success = filters.success

    const response = await api.get('/system-logs', { params })
    const data = response.data
    logList.value = data.data || []
    pagination.total = data.total || 0
  } catch (error) {
    ElMessage.error('获取系统日志失败')
  } finally {
    tableLoading.value = false
  }
}

/**
 * 来源变化时清空类型筛选
 */
const handleSourceChange = () => {
  filters.type = ''
}

/**
 * 点击查询按钮
 */
const handleSearch = () => {
  pagination.page = 1
  fetchLogs()
}

/**
 * 重置筛选条件
 */
const handleReset = () => {
  filters.source = ''
  filters.type = ''
  filters.success = ''
  pagination.page = 1
  fetchLogs()
}

/**
 * 每页条数变化
 */
const handleSizeChange = (size) => {
  pagination.pageSize = size
  pagination.page = 1
  fetchLogs()
}

/**
 * 页码变化
 */
const handlePageChange = (page) => {
  pagination.page = page
  fetchLogs()
}

// ==================== 生命周期 ====================

onMounted(() => {
  fetchLogs()
})
</script>

<style scoped>
/* 系统日志页面容器 */
.system-log-page {
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

/* 筛选区域卡片 */
.filter-card {
  margin-bottom: 20px;
}

/* 筛选表单 */
.filter-form {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
}

/* 记录列表卡片 */
.log-card {
  margin-bottom: 20px;
}

/* 分页组件容器 */
.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
