<template>
  <!-- 根组件：根据登录状态显示不同布局 -->
  <!-- 登录页面：仅显示路由视图 -->
  <div v-if="isLoginPage">
    <div v-if="fixedJWTSecret" class="dev-warning-bar">
      ⚠ 当前使用固定 JWT 密钥运行，仅限开发调试，请勿用于生产环境
    </div>
    <router-view />
  </div>

  <!-- 已登录页面：侧边栏 + 顶部栏 + 内容区域 -->
  <div v-else class="app-wrapper">
    <!-- 固定 JWT 密钥警告条 -->
    <div v-if="fixedJWTSecret" class="dev-warning-bar">
      ⚠ 当前使用固定 JWT 密钥运行，仅限开发调试，请勿用于生产环境
    </div>
    <el-container class="app-layout">    <!-- 左侧导航栏 -->
    <el-aside width="200px" class="app-aside">
      <div class="aside-logo">DNS-HA</div>
      <el-menu
        :default-active="activeMenu"
        router
        background-color="#304156"
        text-color="#bfcbd9"
        active-text-color="#409eff"
      >
        <el-menu-item index="/">
          <el-icon><Monitor /></el-icon>
          <span>系统总览</span>
        </el-menu-item>
        <el-menu-item index="/tasks">
          <el-icon><List /></el-icon>
          <span>探测任务</span>
        </el-menu-item>
        <el-menu-item index="/health-monitors">
          <el-icon><Odometer /></el-icon>
          <span>健康监控</span>
        </el-menu-item>
        <el-menu-item index="/pools">
          <el-icon><Collection /></el-icon>
          <span>解析池</span>
        </el-menu-item>
        <el-menu-item index="/credentials">
          <el-icon><Key /></el-icon>
          <span>凭证管理</span>
        </el-menu-item>
        <el-menu-item index="/notifications/settings">
          <el-icon><Bell /></el-icon>
          <span>通知设置</span>
        </el-menu-item>
        <el-menu-item index="/system-logs">
          <el-icon><Document /></el-icon>
          <span>系统日志</span>
        </el-menu-item>
        <el-menu-item index="/account">
          <el-icon><UserFilled /></el-icon>
          <span>账户管理</span>
        </el-menu-item>
      </el-menu>
    </el-aside>

    <!-- 右侧主区域 -->
    <el-container>
      <!-- 顶部栏 -->
      <el-header class="app-header">
        <span class="header-title">DNSHealth 健康检测解析</span>
        <div class="header-right">
          <span class="version-tag">v0.3.1</span>
          <a href="https://afdian.com/a/xiaozhuhouses" target="_blank" rel="noopener noreferrer" class="sponsor-link" title="赞助支持">
            <el-icon :size="18"><Coffee /></el-icon>
            <span class="sponsor-text">赞助</span>
          </a>
          <a href="https://github.com/yxsj245/DNS-Health" target="_blank" rel="noopener noreferrer" class="github-link" title="GitHub">
            <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor">
              <path d="M12 0C5.37 0 0 5.37 0 12c0 5.31 3.435 9.795 8.205 11.385.6.105.825-.255.825-.57 0-.285-.015-1.23-.015-2.235-3.015.555-3.795-.735-4.035-1.41-.135-.345-.72-1.41-1.23-1.695-.42-.225-1.02-.78-.015-.795.945-.015 1.62.87 1.845 1.23 1.08 1.815 2.805 1.305 3.495.99.105-.78.42-1.305.765-1.605-2.67-.3-5.46-1.335-5.46-5.925 0-1.305.465-2.385 1.23-3.225-.12-.3-.54-1.53.12-3.18 0 0 1.005-.315 3.3 1.23.96-.27 1.98-.405 3-.405s2.04.135 3 .405c2.295-1.56 3.3-1.23 3.3-1.23.66 1.65.24 2.88.12 3.18.765.84 1.23 1.905 1.23 3.225 0 4.605-2.805 5.625-5.475 5.925.435.375.81 1.095.81 2.22 0 1.605-.015 2.895-.015 3.3 0 .315.225.69.825.57A12.02 12.02 0 0024 12c0-6.63-5.37-12-12-12z"/>
            </svg>
          </a>
          <el-button type="danger" size="small" @click="handleLogout">
            退出登录
          </el-button>
        </div>
      </el-header>

      <!-- 内容区域 -->
      <el-main class="app-main">
        <router-view />
      </el-main>
    </el-container>
  </el-container>

    <!-- 赞助弹窗：使用超过24小时后弹出 -->
    <el-dialog
      v-model="showSponsorDialog"
      title="感谢您的使用 ❤️"
      width="420px"
      :close-on-click-modal="true"
      align-center
    >
      <div class="sponsor-dialog-content">
        <p>感谢您持续使用 DNSHealth 健康检测解析系统！</p>
        <p>如果本项目对您有所帮助，欢迎通过爱发电赞助支持开发者继续维护和改进。</p>
        <p>您的每一份支持都是我们前进的动力 🙏</p>
      </div>
      <template #footer>
        <el-button @click="showSponsorDialog = false">稍后再说</el-button>
        <el-button type="primary" @click="openSponsorPage">前往赞助</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Monitor, List, Key, UserFilled, Collection, Bell, Document, Coffee, Odometer } from '@element-plus/icons-vue'
import api from './api'

// ==================== 路由 ====================

const route = useRoute()
const router = useRouter()

// ==================== 开发模式检测 ====================

/** 是否使用了固定 JWT 密钥（开发模式） */
const fixedJWTSecret = ref(false)

/** 是否显示赞助弹窗 */
const showSponsorDialog = ref(false)

/** 24小时使用检测阈值（毫秒） */
const SPONSOR_THRESHOLD_MS = 24 * 60 * 60 * 1000

onMounted(async () => {
  try {
    const res = await api.get('/system-info')
    fixedJWTSecret.value = res.data.fixed_jwt_secret === true
  } catch {
    // 获取失败则忽略，不影响正常使用
  }

  // 赞助弹窗逻辑：记录首次使用时间，超过24小时后弹出
  checkSponsorDialog()
})

/**
 * 检查是否需要弹出赞助弹窗
 * 首次访问时记录时间戳，之后每次访问检查是否超过24小时
 */
const checkSponsorDialog = () => {
  const firstVisitKey = 'dns_health_first_visit'
  const sponsorShownKey = 'dns_health_sponsor_shown'

  const firstVisit = localStorage.getItem(firstVisitKey)
  const sponsorShown = localStorage.getItem(sponsorShownKey)

  if (!firstVisit) {
    // 首次访问，记录时间
    localStorage.setItem(firstVisitKey, Date.now().toString())
    return
  }

  // 已经弹出过则不再弹出
  if (sponsorShown) return

  // 检查是否超过24小时
  const elapsed = Date.now() - parseInt(firstVisit)
  if (elapsed >= SPONSOR_THRESHOLD_MS) {
    showSponsorDialog.value = true
    localStorage.setItem(sponsorShownKey, 'true')
  }
}

/**
 * 打开赞助页面
 */
const openSponsorPage = () => {
  window.open('https://afdian.com/a/xiaozhuhouses', '_blank')
  showSponsorDialog.value = false
}

// ==================== 计算属性 ====================

/**
 * 判断当前是否在登录页面
 * 登录页面不显示侧边栏和顶部栏
 */
const isLoginPage = computed(() => {
  return route.path === '/login' || route.path === '/register'
})

/**
 * 当前激活的菜单项
 * 根据路由路径匹配菜单高亮
 */
const activeMenu = computed(() => {
  const path = route.path
  // 任务相关路径统一高亮"探测任务"菜单
  if (path.startsWith('/tasks')) {
    return '/tasks'
  }
  // 解析池相关路径统一高亮"解析池"菜单
  if (path.startsWith('/pools')) {
    return '/pools'
  }
  // 健康监控相关路径统一高亮"健康监控"菜单
  if (path.startsWith('/health-monitors')) {
    return '/health-monitors'
  }
  if (path.startsWith('/credentials')) {
    return '/credentials'
  }
  // 通知设置路径匹配
  if (path.startsWith('/notifications/settings')) {
    return '/notifications/settings'
  }
  // 系统日志路径匹配
  if (path.startsWith('/system-logs')) {
    return '/system-logs'
  }
  if (path.startsWith('/account')) {
    return '/account'
  }
  return '/'
})

// ==================== 事件处理 ====================

/**
 * 退出登录
 * 调用 POST /api/logout，清除 token，跳转到登录页
 */
const handleLogout = async () => {
  try {
    await api.post('/logout')
  } catch (error) {
    // 即使登出接口失败，也继续清除本地状态
  }
  // 清除本地存储的 token
  localStorage.removeItem('token')
  ElMessage.success('已退出登录')
  // 跳转到登录页
  router.push('/login')
}
</script>

<style>
/* 全局样式：去除默认边距 */
html, body, #app {
  margin: 0;
  padding: 0;
  height: 100%;
}
</style>

<style scoped>
/* 应用整体布局 */
.app-wrapper {
  height: 100vh;
  display: flex;
  flex-direction: column;
}

.app-layout {
  flex: 1;
  overflow: hidden;
}

/* 左侧导航栏 */
.app-aside {
  background-color: #304156;
  overflow-y: auto;
}

/* 侧边栏 Logo 区域 */
.aside-logo {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #ffffff;
  font-size: 18px;
  font-weight: 700;
  background-color: #263445;
}

/* 顶部栏 */
.app-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background-color: #ffffff;
  border-bottom: 1px solid #e6e6e6;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
}

/* 顶部标题 */
.header-title {
  font-size: 18px;
  font-weight: 600;
  color: #303133;
}

/* 顶部栏右侧区域 */
.header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

/* 版本号标签 */
.version-tag {
  font-size: 12px;
  color: #909399;
  background-color: #f4f4f5;
  padding: 2px 8px;
  border-radius: 4px;
}

/* GitHub 图标链接 */
.github-link {
  color: #606266;
  display: flex;
  align-items: center;
  transition: color 0.2s;
}

.github-link:hover {
  color: #409eff;
}

/* 赞助链接 */
.sponsor-link {
  display: flex;
  align-items: center;
  gap: 4px;
  color: #e6a23c;
  text-decoration: none;
  padding: 4px 10px;
  border-radius: 4px;
  transition: all 0.2s;
  font-size: 13px;
  background-color: #fdf6ec;
  border: 1px solid #f5dab1;
}

.sponsor-link:hover {
  background-color: #e6a23c;
  color: #ffffff;
  border-color: #e6a23c;
}

.sponsor-text {
  font-weight: 500;
}

/* 赞助弹窗内容 */
.sponsor-dialog-content {
  text-align: center;
  line-height: 1.8;
  color: #606266;
  font-size: 14px;
}

/* 内容区域 */
.app-main {
  background-color: #f0f2f5;
  overflow-y: auto;
}

/* 开发模式警告条 */
.dev-warning-bar {
  width: 100%;
  background-color: #f56c6c;
  color: #ffffff;
  text-align: center;
  padding: 8px 0;
  font-size: 14px;
  font-weight: 600;
  letter-spacing: 1px;
  flex-shrink: 0;
}
</style>
