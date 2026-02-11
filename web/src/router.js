import { createRouter, createWebHistory } from 'vue-router'

// 路由懒加载，按需加载页面组件
const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('./views/Login.vue'),
    meta: { requiresAuth: false }
  },
  {
    path: '/register',
    name: 'Register',
    component: () => import('./views/Register.vue'),
    meta: { requiresAuth: false }
  },
  {
    path: '/',
    name: 'Dashboard',
    component: () => import('./views/Dashboard.vue'),
    meta: { requiresAuth: true }
  },
  {
    path: '/tasks',
    name: 'TaskList',
    component: () => import('./views/TaskList.vue'),
    meta: { requiresAuth: true }
  },
  {
    path: '/tasks/new',
    name: 'TaskCreate',
    component: () => import('./views/TaskForm.vue'),
    meta: { requiresAuth: true }
  },
  {
    path: '/tasks/:id/edit',
    name: 'TaskEdit',
    component: () => import('./views/TaskForm.vue'),
    meta: { requiresAuth: true }
  },
  {
    path: '/tasks/:id',
    name: 'TaskDetail',
    component: () => import('./views/TaskDetail.vue'),
    meta: { requiresAuth: true }
  },
  // 解析池相关路由
  {
    path: '/pools',
    name: 'PoolList',
    component: () => import('./views/PoolList.vue'),
    meta: { requiresAuth: true }
  },
  {
    path: '/pools/new',
    name: 'PoolCreate',
    component: () => import('./views/PoolForm.vue'),
    meta: { requiresAuth: true }
  },
  {
    path: '/pools/:id/edit',
    name: 'PoolEdit',
    component: () => import('./views/PoolForm.vue'),
    meta: { requiresAuth: true }
  },
  {
    path: '/pools/:id',
    name: 'PoolDetail',
    component: () => import('./views/PoolDetail.vue'),
    meta: { requiresAuth: true }
  },
  {
    path: '/credentials',
    name: 'Credentials',
    component: () => import('./views/Credentials.vue'),
    meta: { requiresAuth: true }
  },
  {
    path: '/account',
    name: 'Account',
    component: () => import('./views/Account.vue'),
    meta: { requiresAuth: true }
  },
  // 通知相关路由
  {
    path: '/notifications/settings',
    name: 'NotificationSettings',
    component: () => import('./views/NotificationSettings.vue'),
    meta: { requiresAuth: true }
  },
  {
    path: '/system-logs',
    name: 'SystemLogs',
    component: () => import('./views/NotificationLog.vue'),
    meta: { requiresAuth: true }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// 导航守卫：未登录时重定向到登录页
router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('token')
  // 如果目标页面需要认证且没有 token，则跳转到登录页
  if (to.meta.requiresAuth !== false && !token) {
    next('/login')
  } else {
    next()
  }
})

export default router
