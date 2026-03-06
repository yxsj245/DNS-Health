<template>
  <!-- 云服务商凭证管理页面 -->
  <div class="credentials-page">
    <!-- 页面头部：标题和添加按钮 -->
    <div class="page-header">
      <h2 class="page-title">云服务商凭证管理</h2>
      <el-button type="primary" @click="showAddDialog">
        添加凭证
      </el-button>
    </div>

    <!-- 凭证列表表格 -->
    <el-table
      :data="credentials"
      v-loading="tableLoading"
      border
      stripe
      style="width: 100%"
      empty-text="暂无凭证数据"
    >
      <el-table-column prop="name" label="名称" min-width="120" />
      <el-table-column prop="provider_type" label="服务商类型" min-width="100">
        <template #default="{ row }">
          {{ getProviderLabel(row.provider_type) }}
        </template>
      </el-table-column>
      <!-- 动态凭证字段列：根据每行的 credentials map 展示 -->
      <el-table-column label="凭证信息（脱敏）" min-width="300">
        <template #default="{ row }">
          <div v-if="row.credentials" class="credential-fields">
            <span
              v-for="(value, key) in row.credentials"
              :key="key"
              class="credential-field"
            >
              <span class="field-label">{{ formatFieldLabel(row.provider_type, key) }}:</span>
              <span class="field-value">{{ value }}</span>
            </span>
          </div>
          <span v-else>-</span>
        </template>
      </el-table-column>
      <el-table-column prop="created_at" label="创建时间" min-width="160" />
      <el-table-column label="操作" width="160" fixed="right">
        <template #default="{ row }">
          <el-button
            type="primary"
            size="small"
            @click="showEditDialog(row)"
          >
            编辑
          </el-button>
          <el-button
            type="danger"
            size="small"
            @click="handleDelete(row)"
          >
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 添加凭证对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEditing ? '编辑凭证' : '添加凭证'"
      width="500px"
      :close-on-click-modal="false"
      @closed="resetForm"
    >
      <el-form
        ref="formRef"
        :model="form"
        :rules="formRules"
        label-width="120px"
      >
        <!-- 凭证名称 -->
        <el-form-item label="凭证名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入凭证名称" />
        </el-form-item>

        <!-- 服务商类型（切换后动态渲染凭证字段） -->
        <el-form-item label="服务商类型" prop="provider_type">
          <el-select
            v-model="form.provider_type"
            placeholder="请选择服务商类型"
            style="width: 100%"
            :disabled="isEditing"
            @change="onProviderChange"
          >
            <el-option
              v-for="opt in providerOptions"
              :key="opt.value"
              :label="opt.label"
              :value="opt.value"
            />
          </el-select>
        </el-form-item>

        <!-- 密钥管理页面链接：根据选中的服务商显示 -->
        <div v-if="currentKeyUrl" class="key-url-hint">
          <el-icon><Link /></el-icon>
          <a :href="currentKeyUrl.url" target="_blank" rel="noopener noreferrer">
            {{ currentKeyUrl.text }}
          </a>
        </div>

        <!-- 动态凭证字段：根据选中的服务商渲染 -->
        <template v-if="currentFields.length > 0">
          <el-form-item
            v-for="field in currentFields"
            :key="field.key"
            :label="field.label"
            :prop="'credentials.' + field.key"
            :rules="(!isEditing && field.required) ? [{ required: true, message: '请输入' + field.label, trigger: 'blur' }] : []"
          >
            <el-input
              v-model="form.credentials[field.key]"
              :type="field.type === 'password' ? 'password' : 'text'"
              :placeholder="isEditing ? '留空则不修改' : field.placeholder"
              :show-password="field.type === 'password'"
            />
          </el-form-item>
        </template>
      </el-form>

      <!-- 对话框底部按钮 -->
      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitLoading" @click="handleSubmit">
          确定
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, reactive, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Link } from '@element-plus/icons-vue'
import api from '../api'
import { getProviderOptions, getProviderFields, getProviderLabel, getProviderKeyUrl } from '../providerConfig'

// ==================== 状态定义 ====================

const credentials = ref([])
const tableLoading = ref(false)
const dialogVisible = ref(false)
const submitLoading = ref(false)
const formRef = ref(null)
const isEditing = ref(false)
const editingId = ref(null)

// 服务商下拉选项
const providerOptions = getProviderOptions()

// 表单数据
const form = reactive({
  name: '',
  provider_type: '',
  credentials: {}
})

// 基础验证规则（动态字段的规则在模板中通过 :rules 绑定）
const formRules = reactive({
  name: [{ required: true, message: '请输入凭证名称', trigger: 'blur' }],
  provider_type: [{ required: true, message: '请选择服务商类型', trigger: 'change' }]
})

// 当前选中服务商的字段配置
const currentFields = computed(() => {
  if (!form.provider_type) return []
  return getProviderFields(form.provider_type) || []
})

// 当前选中服务商的密钥管理链接
const currentKeyUrl = computed(() => {
  if (!form.provider_type) return null
  return getProviderKeyUrl(form.provider_type)
})

// ==================== 辅助函数 ====================

/**
 * 格式化凭证字段标签（列表展示用）
 * 优先从 provider 配置中取中文标签，找不到则直接显示 key
 */
const formatFieldLabel = (providerType, key) => {
  const fields = getProviderFields(providerType)
  if (fields) {
    const field = fields.find(f => f.key === key)
    if (field) return field.label
  }
  return key
}

// ==================== 事件处理 ====================

/**
 * 服务商类型切换时，重置凭证字段
 */
const onProviderChange = () => {
  form.credentials = {}
}

const fetchCredentials = async () => {
  tableLoading.value = true
  try {
    const response = await api.get('/credentials')
    credentials.value = response.data
  } catch (error) {
    ElMessage.error('获取凭证列表失败')
  } finally {
    tableLoading.value = false
  }
}

const showAddDialog = () => {
  isEditing.value = false
  editingId.value = null
  dialogVisible.value = true
}

/**
 * 显示编辑凭证对话框
 * 填充已有数据，服务商类型不可变更，凭证字段需要重新输入
 */
const showEditDialog = (row) => {
  isEditing.value = true
  editingId.value = row.id
  form.name = row.name
  form.provider_type = row.provider_type
  // 凭证字段清空，需要用户重新输入（因为后端返回的是脱敏数据）
  form.credentials = {}
  dialogVisible.value = true
}

const resetForm = () => {
  form.name = ''
  form.provider_type = ''
  form.credentials = {}
  isEditing.value = false
  editingId.value = null
  if (formRef.value) {
    formRef.value.resetFields()
  }
}

const handleSubmit = async () => {
  if (!formRef.value) return
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitLoading.value = true
  try {
    if (isEditing.value) {
      // 编辑模式：PUT 请求，不传 provider_type
      await api.put(`/credentials/${editingId.value}`, {
        name: form.name,
        credentials: { ...form.credentials }
      })
      ElMessage.success('凭证更新成功')
    } else {
      // 新增模式：POST 请求
      await api.post('/credentials', {
        provider_type: form.provider_type,
        name: form.name,
        credentials: { ...form.credentials }
      })
      ElMessage.success('凭证添加成功')
    }
    dialogVisible.value = false
    await fetchCredentials()
  } catch (error) {
    ElMessage.error(isEditing.value ? '更新凭证失败' : '添加凭证失败')
  } finally {
    submitLoading.value = false
  }
}

const handleDelete = async (row) => {
  try {
    await ElMessageBox.confirm(
      `确定要删除凭证「${row.name}」吗？删除后不可恢复。`,
      '删除确认',
      {
        confirmButtonText: '确定删除',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    tableLoading.value = true
    try {
      await api.delete(`/credentials/${row.id}`)
      ElMessage.success('凭证已删除')
      await fetchCredentials()
    } catch (error) {
      ElMessage.error('删除凭证失败')
      tableLoading.value = false
    }
  } catch {
    // 用户取消
  }
}

// ==================== 生命周期 ====================

onMounted(() => {
  fetchCredentials()
})
</script>

<style scoped>
.credentials-page {
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

/* 凭证字段展示样式 */
.credential-fields {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.credential-field {
  font-size: 13px;
  line-height: 1.4;
}

.field-label {
  color: #909399;
  margin-right: 4px;
}

.field-value {
  color: #303133;
  font-family: monospace;
}

/* 密钥管理链接样式 */
.key-url-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  margin: 0 0 18px 120px;
  background-color: #ecf5ff;
  border-radius: 4px;
  font-size: 13px;
  color: #409eff;
}

.key-url-hint a {
  color: #409eff;
  text-decoration: none;
}

.key-url-hint a:hover {
  text-decoration: underline;
  color: #337ecc;
}
</style>
