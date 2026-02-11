import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import router from './router'

// 创建 Vue 应用实例
const app = createApp(App)

// 注册 Element Plus UI 组件库
app.use(ElementPlus)

// 注册路由
app.use(router)

// 挂载应用
app.mount('#app')
