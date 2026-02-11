<template>
  <!-- 探测任务表单页面：创建模式使用分步引导，编辑模式直接显示表单 -->
  <div class="task-form-page">
    <!-- 页面头部 -->
    <div class="page-header">
      <h2 class="page-title">{{ isEditMode ? '编辑探测任务' : '创建探测任务' }}</h2>
    </div>

    <!-- 创建模式：分步引导 -->
    <div v-if="!isEditMode" v-loading="pageLoading">
      <!-- 步骤条 -->
      <el-steps :active="currentStep" finish-status="success" align-center class="wizard-steps">
        <el-step title="任务类型" description="选择故障转移方式" />
        <el-step title="记录类型" description="选择DNS记录类型" />
        <el-step v-if="form.task_type === 'switch' || form.task_type === 'cdn_switch'" title="回切策略" description="选择故障恢复后的处理方式" />
        <el-step title="任务配置" description="填写详细参数" />
      </el-steps>

      <!-- 步骤 1：选择任务类型 -->
      <div v-if="currentStep === 0" class="wizard-content">
        <h3 class="step-title">请选择任务类型</h3>
        <p class="step-desc">任务类型决定了当探测目标异常时，系统将如何处理对应的 DNS 解析记录。</p>
        <div class="type-cards">
          <div
            class="type-card"
            :class="{ active: form.task_type === 'pause_delete' }"
            @click="form.task_type = 'pause_delete'"
          >
            <div class="type-card-icon"><el-icon><CircleClose /></el-icon></div>
            <div class="type-card-title">暂停 / 删除</div>
            <div class="type-card-desc">
              当探测目标连续失败达到阈值时，自动暂停或删除对应的 DNS 解析记录，使流量不再指向故障节点。恢复后自动添加回解析记录。
            </div>
          </div>
          <div
            class="type-card"
            :class="{ active: form.task_type === 'switch' }"
            @click="form.task_type = 'switch'"
          >
            <div class="type-card-icon"><el-icon><Switch /></el-icon></div>
            <div class="type-card-title">切换解析</div>
            <div class="type-card-desc">
              当探测目标连续失败达到阈值时，自动将 DNS 解析切换到备用解析池中的健康资源。支持自动回切或保持当前解析，适合需要高可用切换的场景。
            </div>
          </div>
          <!-- CDN 故障转移：仅 Cloudflare 凭证时显示 -->
          <div
            v-if="isCloudflareCredential"
            class="type-card"
            :class="{ active: form.task_type === 'cdn_switch' }"
            @click="form.task_type = 'cdn_switch'"
          >
            <div class="type-card-icon"><el-icon><Connection /></el-icon></div>
            <div class="type-card-title">CDN 故障转移</div>
            <div class="type-card-desc">
              通过探测 CNAME 域名解析出的 IP 健康状态，当连续失败达到阈值时，自动启用 Cloudflare CDN 代理（橙色云）并将记录值切换为指定的目标 IP。恢复后可自动关闭 CDN 代理并恢复原始记录值，仅限 Cloudflare 服务商使用。
            </div>
          </div>
        </div>
        <div class="wizard-actions">
          <el-button @click="handleCancel">取消</el-button>
          <el-button type="primary" @click="handleStep1Next">下一步</el-button>
        </div>
      </div>

      <!-- 步骤 2：选择记录类型 -->
      <div v-if="currentStep === 1" class="wizard-content">
        <h3 class="step-title">请选择记录类型</h3>
        <p class="step-desc">记录类型决定了系统监控和操作的 DNS 记录种类，不同记录类型的探测和故障转移逻辑有所不同。</p>
        <div class="type-cards">
          <div
            class="type-card"
            :class="{ active: form.record_type === 'A_AAAA' }"
            @click="form.record_type = 'A_AAAA'"
          >
            <div class="type-card-icon"><el-icon><Position /></el-icon></div>
            <div class="type-card-title">A / AAAA</div>
            <div class="type-card-desc">
              直接指向 IP 地址的解析记录。系统将逐一探测域名下的所有 IP，对不健康的 IP 执行暂停/删除或切换操作。适用于域名直接解析到服务器 IP 的场景。
            </div>
          </div>
          <div
            class="type-card"
            :class="{ active: form.record_type === 'CNAME' }"
            @click="form.record_type = 'CNAME'"
          >
            <div class="type-card-icon"><el-icon><Link /></el-icon></div>
            <div class="type-card-title">CNAME</div>
            <div class="type-card-desc">
              指向另一个域名的别名记录。系统将解析 CNAME 目标域名获取实际 IP 列表，并探测这些 IP 的健康状态。当不健康 IP 达到阈值时触发故障转移操作。
            </div>
          </div>
        </div>
        <div class="wizard-actions">
          <el-button @click="currentStep = 0">上一步</el-button>
          <el-button type="primary" @click="handleStep2Next">下一步</el-button>
        </div>
      </div>

      <!-- 回切策略步骤（切换解析或CDN故障转移时显示） -->
      <div v-if="currentStep === switchBackStep && (form.task_type === 'switch' || form.task_type === 'cdn_switch')" class="wizard-content">
        <h3 class="step-title">请选择回切策略</h3>
        <p class="step-desc">回切策略决定了当原始解析目标恢复健康后，系统是否自动切换回原始解析。</p>
        <div class="type-cards">
          <div
            class="type-card"
            :class="{ active: form.switch_back_policy === 'auto' }"
            @click="form.switch_back_policy = 'auto'"
          >
            <div class="type-card-icon"><el-icon><RefreshRight /></el-icon></div>
            <div class="type-card-title">自动回切</div>
            <div class="type-card-desc">
              当原始解析目标恢复健康并达到恢复阈值后，系统将自动切换回原始解析记录，确保流量回到主要节点。适合需要优先使用主节点的场景。
            </div>
          </div>
          <div
            class="type-card"
            :class="{ active: form.switch_back_policy === 'manual' }"
            @click="form.switch_back_policy = 'manual'"
          >
            <div class="type-card-icon"><el-icon><Lock /></el-icon></div>
            <div class="type-card-title">保持当前</div>
            <div class="type-card-desc">
              即使原始解析目标恢复健康，系统也不会自动切换回去，保持当前的备用解析。需要手动干预才能切换回原始解析。适合需要稳定性优先的场景。
            </div>
          </div>
        </div>
        <div class="wizard-actions">
          <el-button @click="currentStep = switchBackStep - 1">上一步</el-button>
          <el-button type="primary" @click="currentStep = formStep">下一步</el-button>
        </div>
      </div>

      <!-- 任务配置表单步骤（分组卡片布局） -->
      <div v-if="currentStep === formStep" class="wizard-content wizard-form-content">
        <el-form
          ref="formRef"
          :model="form"
          :rules="formRules"
          label-width="120px"
        >
          <!-- 已选配置摘要 -->
          <div class="config-summary">
            <el-tag type="primary" effect="dark" size="large">
              {{ form.task_type === 'switch' ? '切换解析' : (form.task_type === 'cdn_switch' ? 'CDN 故障转移' : '暂停/删除') }}
            </el-tag>
            <span class="summary-divider">+</span>
            <el-tag type="success" effect="dark" size="large">
              {{ form.record_type === 'CNAME' ? 'CNAME' : 'A/AAAA' }}
            </el-tag>
            <template v-if="form.task_type === 'switch' || form.task_type === 'cdn_switch'">
              <span class="summary-divider">+</span>
              <el-tag type="warning" effect="dark" size="large">
                {{ form.switch_back_policy === 'auto' ? '自动回切' : '保持当前' }}
              </el-tag>
            </template>
            <el-button link type="primary" class="summary-change" @click="currentStep = 0">修改选择</el-button>
          </div>

          <!-- 故障转移配置（条件显示） -->
          <el-card v-if="showPoolField || showCdnSwitchField || showCnameThresholdField" class="form-section" shadow="never">
            <template #header>
              <div class="section-header">
                <el-icon><Switch /></el-icon>
                <span>{{ showCdnSwitchField ? 'CDN 故障转移配置' : (showPoolField ? '故障转移配置' : 'CNAME 阈值配置') }}</span>
              </div>
            </template>
            <el-form-item v-if="showPoolField" label="解析池" prop="pool_id">
              <el-select v-model="form.pool_id" placeholder="请选择解析池" style="width: 100%">
                <el-option v-for="pool in pools" :key="pool.id" :label="pool.name" :value="pool.id" />
              </el-select>
            </el-form-item>
            <!-- CDN 故障转移：目标 IP 输入 -->
            <el-form-item v-if="showCdnSwitchField" label="目标 IP" prop="cdn_target">
              <el-input v-model="form.cdn_target" placeholder="请输入故障转移时切换到的目标 IP 地址" />
            </el-form-item>
            <el-form-item v-if="showCnameThresholdField" label="阈值类型" prop="fail_threshold_type">
              <el-select v-model="form.fail_threshold_type" placeholder="请选择阈值类型" style="width: 100%">
                <el-option label="个数" value="count" />
                <el-option label="百分比" value="percent" />
              </el-select>
            </el-form-item>
            <el-form-item v-if="showCnameThresholdField" label="阈值数值" prop="fail_threshold_value">
              <!-- 百分比类型使用滑动条 -->
              <el-slider
                v-if="form.fail_threshold_type === 'percent'"
                v-model="form.fail_threshold_value"
                :min="1"
                :max="100"
                :show-tooltip="true"
                :format-tooltip="(val) => val + '%'"
                style="width: 100%"
              />
              <!-- 个数类型使用数字输入框 -->
              <el-input-number
                v-else
                v-model="form.fail_threshold_value"
                :min="1"
                style="width: 100%"
              />
            </el-form-item>
          </el-card>

          <!-- 域名配置 -->
          <el-card class="form-section" shadow="never">
            <template #header>
              <div class="section-header">
                <el-icon><Position /></el-icon>
                <span>域名配置</span>
              </div>
            </template>
            <el-form-item label="域名" prop="domain">
              <el-input v-model="form.domain" placeholder="请输入域名，例如 example.com" />
            </el-form-item>
            <el-form-item label="主机记录" prop="sub_domain">
              <el-input v-model="form.sub_domain" placeholder="@ 表示根域名" />
            </el-form-item>
            <el-form-item label="凭证" prop="credential_id">
              <el-select v-model="form.credential_id" placeholder="请选择云服务商凭证" style="width: 100%">
                <el-option v-for="cred in credentials" :key="cred.id" :label="cred.name" :value="cred.id" />
              </el-select>
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
                  <el-tooltip content="连续探测失败达到此次数后，将执行故障转移操作" placement="top">
                    <el-input-number v-model="form.fail_threshold" :min="1" style="width: 100%" />
                  </el-tooltip>
                </el-form-item>
              </el-col>
              <el-col :span="12">
                <el-form-item label="恢复阈值" prop="recover_threshold">
                  <el-tooltip content="连续探测成功达到此次数后，将恢复 DNS 解析记录" placement="top">
                    <el-input-number v-model="form.recover_threshold" :min="1" style="width: 100%" />
                  </el-tooltip>
                </el-form-item>
              </el-col>
            </el-row>
          </el-card>

          <!-- 操作按钮 -->
          <div class="wizard-actions form-actions">
            <div>
              <el-button @click="handleFormPrevStep">上一步</el-button>
              <el-button @click="handleCancel">取消</el-button>
            </div>
            <el-button type="primary" :loading="submitLoading" @click="handleSubmit">创建任务</el-button>
          </div>
        </el-form>
      </div>
    </div>

    <!-- 编辑模式：分组卡片布局 -->
    <div v-if="isEditMode" class="wizard-content wizard-form-content" v-loading="pageLoading">
      <el-form
        ref="formRef"
        :model="form"
        :rules="formRules"
        label-width="120px"
      >
        <!-- 已选配置摘要（只读） -->
        <div class="config-summary">
          <el-tag type="primary" effect="dark" size="large">
            {{ form.task_type === 'switch' ? '切换解析' : (form.task_type === 'cdn_switch' ? 'CDN 故障转移' : '暂停/删除') }}
          </el-tag>
          <span class="summary-divider">+</span>
          <el-tag type="success" effect="dark" size="large">
            {{ form.record_type === 'CNAME' ? 'CNAME' : 'A/AAAA' }}
          </el-tag>
          <template v-if="form.task_type === 'switch' || form.task_type === 'cdn_switch'">
            <span class="summary-divider">+</span>
            <el-tag type="warning" effect="dark" size="large">
              {{ form.switch_back_policy === 'auto' ? '自动回切' : '保持当前' }}
            </el-tag>
          </template>
        </div>

        <!-- 故障转移配置（条件显示） -->
        <el-card v-if="showPoolField || showSwitchBackField || showCdnSwitchField || showCnameThresholdField" class="form-section" shadow="never">
          <template #header>
            <div class="section-header">
              <el-icon><Switch /></el-icon>
              <span>{{ showCdnSwitchField ? 'CDN 故障转移配置' : (showPoolField ? '故障转移配置' : 'CNAME 阈值配置') }}</span>
            </div>
          </template>
          <el-form-item v-if="showPoolField" label="解析池" prop="pool_id">
            <el-select v-model="form.pool_id" placeholder="请选择解析池" style="width: 100%">
              <el-option v-for="pool in pools" :key="pool.id" :label="pool.name" :value="pool.id" />
            </el-select>
          </el-form-item>
          <!-- CDN 故障转移：目标 IP 输入 -->
          <el-form-item v-if="showCdnSwitchField" label="目标 IP" prop="cdn_target">
            <el-input v-model="form.cdn_target" placeholder="请输入故障转移时切换到的目标 IP 地址" />
          </el-form-item>
          <el-form-item v-if="showSwitchBackField" label="回切策略" prop="switch_back_policy">
            <el-select v-model="form.switch_back_policy" placeholder="请选择回切策略" style="width: 100%">
              <el-option label="自动回切" value="auto" />
              <el-option label="保持当前" value="manual" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="showCnameThresholdField" label="阈值类型" prop="fail_threshold_type">
            <el-select v-model="form.fail_threshold_type" placeholder="请选择阈值类型" style="width: 100%">
              <el-option label="个数" value="count" />
              <el-option label="百分比" value="percent" />
            </el-select>
          </el-form-item>
          <el-form-item v-if="showCnameThresholdField" label="阈值数值" prop="fail_threshold_value">
            <!-- 百分比类型使用滑动条 -->
            <el-slider
              v-if="form.fail_threshold_type === 'percent'"
              v-model="form.fail_threshold_value"
              :min="1"
              :max="100"
              :show-tooltip="true"
              :format-tooltip="(val) => val + '%'"
              style="width: 100%"
            />
            <!-- 个数类型使用数字输入框 -->
            <el-input-number
              v-else
              v-model="form.fail_threshold_value"
              :min="1"
              style="width: 100%"
            />
          </el-form-item>
        </el-card>

        <!-- 域名配置 -->
        <el-card class="form-section" shadow="never">
          <template #header>
            <div class="section-header">
              <el-icon><Position /></el-icon>
              <span>域名配置</span>
            </div>
          </template>
          <el-form-item label="域名" prop="domain">
            <el-input v-model="form.domain" placeholder="请输入域名，例如 example.com" />
          </el-form-item>
          <el-form-item label="主机记录" prop="sub_domain">
            <el-input v-model="form.sub_domain" placeholder="@ 表示根域名" />
          </el-form-item>
          <el-form-item label="凭证" prop="credential_id">
            <el-select v-model="form.credential_id" placeholder="请选择云服务商凭证" style="width: 100%">
              <el-option v-for="cred in credentials" :key="cred.id" :label="cred.name" :value="cred.id" />
            </el-select>
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
                <el-tooltip content="连续探测失败达到此次数后，将执行故障转移操作" placement="top">
                  <el-input-number v-model="form.fail_threshold" :min="1" style="width: 100%" />
                </el-tooltip>
              </el-form-item>
            </el-col>
            <el-col :span="12">
              <el-form-item label="恢复阈值" prop="recover_threshold">
                <el-tooltip content="连续探测成功达到此次数后，将恢复 DNS 解析记录" placement="top">
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
import { ref, reactive, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { CircleClose, Switch, Position, Link, Monitor, RefreshRight, Lock, Connection } from '@element-plus/icons-vue'
import api from '../api'

// ==================== 路由与状态 ====================

const route = useRoute()
const router = useRouter()

// 判断是否为编辑模式：路由中存在 id 参数则为编辑模式
const isEditMode = computed(() => !!route.params.id)

// 当前引导步骤（0: 任务类型, 1: 记录类型, 2: 表单配置）
const currentStep = ref(0)

// 页面加载状态（编辑模式下加载任务数据时使用）
const pageLoading = ref(false)

// 表单提交加载状态
const submitLoading = ref(false)

// 表单引用
const formRef = ref(null)

// 凭证列表（用于下拉选择）
const credentials = ref([])

// 解析池列表（用于下拉选择）
const pools = ref([])

// ==================== 表单数据 ====================

const form = reactive({
  task_type: 'pause_delete',
  record_type: 'A_AAAA',
  domain: '',
  sub_domain: '',
  credential_id: null,
  probe_protocol: '',
  probe_port: 80,
  probe_interval_sec: 60,
  timeout_ms: 3000,
  fail_threshold: 3,
  recover_threshold: 3,
  pool_id: null,
  switch_back_policy: 'auto',
  fail_threshold_type: 'count',
  fail_threshold_value: 1,
  cdn_target: ''
})

// ==================== 计算属性 ====================

/**
 * 是否显示端口字段
 * 仅在 TCP/UDP/HTTP/HTTPS 协议时显示
 */
const showPortField = computed(() => {
  return ['TCP', 'UDP', 'HTTP', 'HTTPS'].includes(form.probe_protocol)
})

/**
 * 是否显示解析池选择器
 * 仅切换解析类型时显示（暂停/删除+CNAME不需要解析池，CDN故障转移也不需要解析池）
 */
const showPoolField = computed(() => {
  return form.task_type === 'switch'
})

/**
 * 是否显示回切策略选择器
 * 编辑模式下，切换解析或CDN故障转移类型时显示
 */
const showSwitchBackField = computed(() => {
  return isEditMode.value && (form.task_type === 'switch' || form.task_type === 'cdn_switch')
})

/**
 * 是否显示 CDN 故障转移配置（CNAME 目标值输入）
 * 仅 cdn_switch 类型时显示
 */
const showCdnSwitchField = computed(() => {
  return form.task_type === 'cdn_switch'
})

/**
 * 是否显示阈值配置（百分比/个数）
 * CNAME记录类型 或 CDN故障转移类型时显示
 */
const showCnameThresholdField = computed(() => {
  return form.record_type === 'CNAME' || form.task_type === 'cdn_switch'
})

/**
 * 获取当前选中凭证的 provider_type
 * 用于判断是否显示 CDN 故障转移选项
 */
const selectedCredentialProviderType = computed(() => {
  if (!form.credential_id) return ''
  const cred = credentials.value.find(c => c.id === form.credential_id)
  return cred ? cred.provider_type : ''
})

/**
 * 是否为 Cloudflare 凭证
 * 创建模式下：凭证列表中存在 Cloudflare 凭证时显示 CDN 故障转移选项
 * 编辑模式下：当前选中凭证为 Cloudflare 时显示
 */
const isCloudflareCredential = computed(() => {
  if (isEditMode.value) {
    return selectedCredentialProviderType.value === 'cloudflare'
  }
  // 创建模式：如果已选凭证则按选中的判断，否则检查是否存在任何 Cloudflare 凭证
  if (form.credential_id) {
    return selectedCredentialProviderType.value === 'cloudflare'
  }
  return credentials.value.some(c => c.provider_type === 'cloudflare')
})

// ==================== 事件处理 ====================

/**
 * 步骤导航计算属性
 * 所有类型都经过记录类型步骤
 */
const switchBackStep = computed(() => {
  // 切换解析/CDN故障转移：步骤0(类型) → 步骤1(记录类型) → 步骤2(回切策略)
  return 2
})

const formStep = computed(() => {
  // 切换解析/CDN故障转移：步骤0(类型) → 步骤1(记录类型) → 步骤2(回切策略) → 步骤3(表单)
  // 暂停/删除：步骤0(类型) → 步骤1(记录类型) → 步骤2(表单)
  if (form.task_type === 'switch' || form.task_type === 'cdn_switch') return 3
  return 2
})

/**
 * 步骤1下一步处理
 * 所有类型都进入记录类型步骤
 */
const handleStep1Next = () => {
  currentStep.value = 1
}

/**
 * 步骤2下一步处理
 * 切换解析或CDN故障转移跳转到回切策略步骤
 * 否则跳转到表单步骤
 */
const handleStep2Next = () => {
  if (form.task_type === 'switch' || form.task_type === 'cdn_switch') {
    currentStep.value = 2 // 到回切策略
  } else {
    currentStep.value = 2 // 到表单
  }
}

/**
 * 任务配置表单上一步处理
 * 如果是切换解析或CDN故障转移类型，返回步骤3（回切策略）
 * 否则返回步骤1（记录类型）
 */
const handleFormPrevStep = () => {
  if (form.task_type === 'switch' || form.task_type === 'cdn_switch') {
    currentStep.value = switchBackStep.value
  } else {
    currentStep.value = 1
  }
}

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

/**
 * 正整数验证器
 */
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
  domain: [
    { required: true, message: '请输入域名', trigger: 'blur' }
  ],
  sub_domain: [
    { required: true, message: '请输入主机记录', trigger: 'blur' }
  ],
  credential_id: [
    { required: true, message: '请选择凭证', trigger: 'change' }
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
  ],
  pool_id: [
    { required: true, message: '请选择解析池', trigger: 'change' }
  ],
  switch_back_policy: [
    { required: true, message: '请选择回切策略', trigger: 'change' }
  ],
  fail_threshold_type: [
    { required: true, message: '请选择阈值类型', trigger: 'change' }
  ],
  fail_threshold_value: [
    { required: true, validator: validatePositiveInt, trigger: 'blur' }
  ],
  cdn_target: [
    { required: true, message: '请输入目标 IP 地址', trigger: 'blur' }
  ]
})

// ==================== API 调用 ====================

/**
 * 获取凭证列表（用于下拉选择）
 */
const fetchCredentials = async () => {
  try {
    const response = await api.get('/credentials')
    credentials.value = response.data
  } catch (error) {
    ElMessage.error('获取凭证列表失败')
  }
}

/**
 * 获取解析池列表（用于下拉选择）
 */
const fetchPools = async () => {
  try {
    const response = await api.get('/pools')
    pools.value = response.data
  } catch (error) {
    ElMessage.error('获取解析池列表失败')
  }
}

/**
 * 加载任务数据（编辑模式）
 */
const fetchTask = async () => {
  const taskId = route.params.id
  pageLoading.value = true
  try {
    const response = await api.get(`/tasks/${taskId}`)
    const task = response.data
    form.domain = task.domain
    form.sub_domain = task.sub_domain
    form.credential_id = task.credential_id
    form.probe_protocol = task.probe_protocol
    form.probe_port = task.probe_port || 80
    form.probe_interval_sec = task.probe_interval_sec
    form.timeout_ms = task.timeout_ms
    form.fail_threshold = task.fail_threshold
    form.recover_threshold = task.recover_threshold
    // 回填新增字段
    form.task_type = task.task_type || 'pause_delete'
    form.record_type = task.record_type || 'A_AAAA'
    // 兼容旧数据：将单独的 A 或 AAAA 映射为 A_AAAA
    if (form.record_type === 'A' || form.record_type === 'AAAA') {
      form.record_type = 'A_AAAA'
    }
    form.pool_id = task.pool_id || null
    form.switch_back_policy = task.switch_back_policy || 'auto'
    form.fail_threshold_type = task.fail_threshold_type || 'count'
    form.fail_threshold_value = task.fail_threshold_value || 1
    form.cdn_target = task.cdn_target || ''
  } catch (error) {
    ElMessage.error('获取任务数据失败')
    router.push('/tasks')
  } finally {
    pageLoading.value = false
  }
}

/**
 * 提交表单
 */
const handleSubmit = async () => {
  if (!formRef.value) return

  // CDN 故障转移时手动验证
  if (form.task_type === 'cdn_switch') {
    if (!form.cdn_target.trim()) {
      ElMessage.error('请输入目标 IP 地址')
      return
    }
  }

  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return

  submitLoading.value = true

  const requestData = {
    credential_id: form.credential_id,
    domain: form.domain,
    sub_domain: form.sub_domain,
    probe_protocol: form.probe_protocol,
    probe_port: showPortField.value ? form.probe_port : 0,
    probe_interval_sec: form.probe_interval_sec,
    timeout_ms: form.timeout_ms,
    fail_threshold: form.fail_threshold,
    recover_threshold: form.recover_threshold,
    task_type: form.task_type,
    record_type: form.record_type,
    pool_id: showPoolField.value ? form.pool_id : null,
    switch_back_policy: (showSwitchBackField.value || form.task_type === 'switch' || form.task_type === 'cdn_switch') ? form.switch_back_policy : '',
    fail_threshold_type: showCnameThresholdField.value ? form.fail_threshold_type : '',
    fail_threshold_value: showCnameThresholdField.value ? form.fail_threshold_value : 0,
    cdn_target: showCdnSwitchField.value ? form.cdn_target : ''
  }

  try {
    if (isEditMode.value) {
      await api.put(`/tasks/${route.params.id}`, requestData)
      ElMessage.success('任务更新成功')
    } else {
      await api.post('/tasks', requestData)
      ElMessage.success('任务创建成功')
    }
    router.push('/tasks')
  } catch (error) {
    const action = isEditMode.value ? '更新' : '创建'
    ElMessage.error(`${action}任务失败`)
  } finally {
    submitLoading.value = false
  }
}

/**
 * 取消操作，返回任务列表
 */
const handleCancel = () => {
  router.push('/tasks')
}

// ==================== 生命周期 ====================

// 监听凭证变化：如果当前选择了 cdn_switch 但凭证不是 Cloudflare，则重置任务类型
watch(() => form.credential_id, () => {
  if (form.task_type === 'cdn_switch' && form.credential_id && selectedCredentialProviderType.value !== 'cloudflare') {
    form.task_type = 'pause_delete'
  }
})

onMounted(async () => {
  await fetchCredentials()
  await fetchPools()
  if (isEditMode.value) {
    await fetchTask()
  }
})
</script>

<style scoped>
/* 任务表单页面容器 */
.task-form-page {
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

/* 步骤条 */
.wizard-steps {
  margin-bottom: 32px;
}

/* 引导内容区域 */
.wizard-content {
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

/* 选中状态 */
.type-card.active {
  border-color: #409eff;
  background: #ecf5ff;
  box-shadow: 0 2px 12px rgba(64, 158, 255, 0.15);
}

/* 卡片图标 */
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

/* 卡片标题 */
.type-card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
  margin-bottom: 8px;
}

/* 卡片描述 */
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

/* 步骤3表单内容区域 */
.wizard-form-content {
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

/* 摘要分隔符 */
.summary-divider {
  color: #909399;
  font-size: 16px;
  font-weight: 600;
}

/* 修改选择链接 */
.summary-change {
  margin-left: auto;
  font-size: 13px;
}

/* 表单分组卡片 */
.form-section {
  margin-bottom: 20px;
}

/* 卡片分组标题 */
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

/* 表单操作按钮区域 */
.form-actions {
  margin-top: 24px;
}

/* 表单单位文字 */
.form-unit {
  margin-left: 8px;
  color: #909399;
  font-size: 14px;
}
</style>
