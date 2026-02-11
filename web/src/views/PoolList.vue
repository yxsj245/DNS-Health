<template>
  <!-- 解析池列表页面 -->
  <div class="pool-list-page">
    <!-- 页面头部：标题和创建按钮 -->
    <div class="page-header">
      <h2 class="page-title">解析池列表</h2>
      <el-button type="primary" @click="handleCreate">
        创建解析池
      </el-button>
    </div>

    <!-- 解析池列表表格 -->
    <el-table
      :data="pools"
      v-loading="tableLoading"
      border
      stripe
      style="width: 100%"
      empty-text="暂无解析池"
    >
      <!-- 池名称列 -->
      <el-table-column prop="name" label="池名称" min-width="180" />

      <!-- 资源类型列 -->
      <el-table-column label="资源类型" min-width="120">
        <template #default="{ row }">
          {{ formatResourceType(row.resource_type) }}
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

      <!-- 操作列 -->
      <el-table-column label="操作" width="260" fixed="right">
        <template #default="{ row }">
          <el-button type="primary" size="small" @click="handleDetail(row)">
            查看详情
          </el-button>
          <el-button type="warning" size="small" @click="handleEdit(row)">
            编辑
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

// 解析池列表数据
const pools = ref([])

// 表格加载状态
const tableLoading = ref(false)

// ==================== 辅助函数 ====================

/**
 * 格式化资源类型显示
 */
const formatResourceType = (type) => {
  if (type === 'ip') return 'IP'
  if (type === 'domain') return '域名'
  return type
}

// ==================== API 调用 ====================

/**
 * 获取解析池列表
 */
const fetchPools = async () => {
  tableLoading.value = true
  try {
    const response = await api.get('/pools')
    pools.value = response.data
  } catch (error) {
    ElMessage.error('获取解析池列表失败')
  } finally {
    tableLoading.value = false
  }
}

/**
 * 跳转到创建解析池页面
 */
const handleCreate = () => {
  router.push('/pools/new')
}

/**
 * 跳转到解析池详情页面
 */
const handleDetail = (row) => {
  router.push(`/pools/${row.id}`)
}

/**
 * 跳转到编辑解析池页面
 */
const handleEdit = (row) => {
  router.push(`/pools/${row.id}/edit`)
}

/**
 * 删除解析池
 */
const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除解析池「${row.name}」吗？删除后不可恢复。`,
      '删除确认',
      {
        confirmButtonText: '确定删除',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )

    tableLoading.value = true
    try {
      await api.delete(`/pools/${row.id}`)
      ElMessage.success('解析池已删除')
      await fetchPools()
    } catch (error) {
      if (error.response && error.response.data && error.response.data.error) {
        ElMessage.error(error.response.data.error)
      } else {
        ElMessage.error('删除解析池失败')
      }
      tableLoading.value = false
    }
  } catch {
    // 用户取消删除
  }
}

// ==================== 生命周期 ====================

onMounted(() => {
  fetchPools()
})
</script>

<style scoped>
.pool-list-page {
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
</style>
