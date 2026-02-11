<template>
  <!-- 延迟曲线图表组件 -->
  <el-card class="latency-chart-card" shadow="never">
    <template #header>
      <div class="chart-header">
        <span class="card-title">延迟曲线图表</span>
        <span class="probe-interval-info">检测周期: {{ probeIntervalSec }}s</span>
      </div>
    </template>

    <!-- IP 列表为空时的提示 -->
    <div v-if="!ipList || ipList.length === 0" class="empty-ip-tip">
      <el-empty description="暂无可选 IP" />
    </div>

    <!-- IP 列表非空时显示筛选器和图表 -->
    <div v-else>
      <!-- 工具栏：IP 选择器 + 日期范围选择器 -->
      <div class="toolbar">
        <el-select
          v-model="selectedIp"
          placeholder="请选择 IP"
          style="width: 220px; margin-right: 16px"
        >
          <el-option
            v-for="ip in ipList"
            :key="ip"
            :label="ip"
            :value="ip"
          />
        </el-select>
        <el-date-picker
          v-model="dateRange"
          type="datetimerange"
          range-separator="至"
          start-placeholder="开始时间"
          end-placeholder="结束时间"
          :disabled-date="disabledDate"
          @change="handleDateChange"
        />
      </div>

      <!-- 图表区域 -->
      <div v-loading="loading" class="chart-container">
        <!-- 有数据时渲染图表 -->
        <v-chart
          v-if="chartData.length > 0"
          :option="chartOption"
          autoresize
          class="chart"
        />
        <!-- 无数据时显示空状态 -->
        <el-empty v-else-if="!loading" description="暂无延迟数据" />
      </div>
    </div>
  </el-card>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  LegendComponent,
  DataZoomComponent
} from 'echarts/components'
import api from '../api'

// 注册 ECharts 组件
use([CanvasRenderer, LineChart, GridComponent, TooltipComponent, LegendComponent, DataZoomComponent])

// ==================== Props 定义 ====================

const props = defineProps({
  // API 路径前缀，如 /tasks/1 或 /health-monitors/1
  apiUrl: {
    type: String,
    required: true
  },
  // IP 地址列表
  ipList: {
    type: Array,
    default: () => []
  },
  // 检测周期（秒）
  probeIntervalSec: {
    type: Number,
    default: 30
  }
})

// ==================== 内部状态 ====================

// 当前选中的 IP
const selectedIp = ref('')
// 日期范围，默认最近 24 小时
const dateRange = ref(getDefaultDateRange())
// 图表原始数据
const chartData = ref([])
// 加载状态
const loading = ref(false)

/**
 * 获取默认日期范围（最近 24 小时）
 * @returns {Array} [开始时间, 结束时间]
 */
function getDefaultDateRange() {
  const end = new Date()
  const start = new Date()
  start.setTime(start.getTime() - 24 * 60 * 60 * 1000)
  return [start, end]
}

// ==================== 日期校验 ====================

/**
 * 禁用日期：不允许选择未来日期
 * @param {Date} date - 待校验日期
 * @returns {boolean} 是否禁用
 */
const disabledDate = (date) => {
  return date.getTime() > Date.now()
}

/**
 * 日期范围变更处理
 * 结束日期不能早于开始日期时，恢复默认范围
 * @param {Array} val - 新的日期范围
 */
const handleDateChange = (val) => {
  if (!val || val.length !== 2) {
    return
  }
  if (val[1] < val[0]) {
    ElMessage.warning('结束日期不能早于开始日期')
    dateRange.value = getDefaultDateRange()
  }
}

// ==================== 数据请求 ====================

/**
 * 从后端获取延迟数据
 * 调用 GET {apiUrl}/latency?ip=xxx&start_time=xxx&end_time=xxx
 */
const fetchLatencyData = async () => {
  if (!selectedIp.value || !dateRange.value || dateRange.value.length !== 2) {
    return
  }

  loading.value = true
  chartData.value = []

  try {
    const [startTime, endTime] = dateRange.value
    const response = await api.get(`${props.apiUrl}/latency`, {
      params: {
        ip: selectedIp.value,
        start_time: startTime.toISOString(),
        end_time: endTime.toISOString()
      }
    })
    chartData.value = response.data?.data || []
  } catch (error) {
    ElMessage.error('获取延迟数据失败')
    chartData.value = []
  } finally {
    loading.value = false
  }
}

// ==================== ECharts 配置 ====================

/**
 * 计算 ECharts 图表配置项
 * X 轴为时间，Y 轴为延迟（ms），失败探测点用红色标记
 */
const chartOption = computed(() => {
  // X 轴时间标签
  const xData = chartData.value.map(item => item.probed_at)
  // Y 轴延迟值
  const yData = chartData.value.map(item => item.latency_ms)
  // 数据点样式：失败的探测用红色标记
  const itemStyles = chartData.value.map(item => {
    if (!item.success) {
      return {
        color: '#F56C6C',
        borderColor: '#F56C6C'
      }
    }
    return null
  })

  return {
    tooltip: {
      trigger: 'axis',
      formatter: (params) => {
        const point = params[0]
        if (!point) return ''
        const dataIndex = point.dataIndex
        const item = chartData.value[dataIndex]
        const status = item?.success ? '成功' : '失败'
        const statusColor = item?.success ? '#67C23A' : '#F56C6C'
        return `
          <div>
            <div>${point.axisValue}</div>
            <div>延迟: ${point.value} ms</div>
            <div>状态: <span style="color:${statusColor}">${status}</span></div>
          </div>
        `
      }
    },
    grid: {
      left: '3%',
      right: '4%',
      bottom: '15%',
      containLabel: true
    },
    xAxis: {
      type: 'category',
      data: xData,
      axisLabel: {
        rotate: 30,
        fontSize: 11
      }
    },
    yAxis: {
      type: 'value',
      name: '延迟 (ms)',
      min: 0
    },
    dataZoom: [
      {
        type: 'inside',
        start: 0,
        end: 100
      },
      {
        type: 'slider',
        start: 0,
        end: 100
      }
    ],
    series: [
      {
        name: '延迟',
        type: 'line',
        smooth: true,
        symbol: 'circle',
        symbolSize: 6,
        itemStyle: {
          color: '#409EFF'
        },
        lineStyle: {
          width: 2
        },
        // 逐点设置样式，失败的探测点用红色
        data: yData.map((val, idx) => ({
          value: val,
          itemStyle: itemStyles[idx]
        }))
      }
    ]
  }
})

// ==================== 监听器 ====================

// 监听 ipList 变化：非空时自动选中第一个 IP
watch(
  () => props.ipList,
  (newList) => {
    if (newList && newList.length > 0) {
      selectedIp.value = newList[0]
    } else {
      selectedIp.value = ''
      chartData.value = []
    }
  },
  { immediate: true }
)

// 监听选中 IP 变化：重新加载数据
watch(selectedIp, (newIp) => {
  if (newIp) {
    fetchLatencyData()
  }
})

// 监听日期范围变化：重新加载数据
watch(dateRange, (newRange) => {
  if (newRange && newRange.length === 2 && selectedIp.value) {
    fetchLatencyData()
  }
})
</script>

<style scoped>
/* 延迟图表卡片 */
.latency-chart-card {
  margin-bottom: 20px;
}

/* 卡片头部 */
.chart-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.card-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

/* 检测周期信息 */
.probe-interval-info {
  font-size: 13px;
  color: #909399;
}

/* 工具栏 */
.toolbar {
  display: flex;
  align-items: center;
  margin-bottom: 16px;
  flex-wrap: wrap;
  gap: 8px;
}

/* 图表容器 */
.chart-container {
  min-height: 350px;
}

/* ECharts 图表 */
.chart {
  width: 100%;
  height: 350px;
}

/* IP 为空时的提示 */
.empty-ip-tip {
  padding: 20px 0;
}
</style>
