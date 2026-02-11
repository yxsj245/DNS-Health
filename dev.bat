@echo off
chcp 65001 >nul
echo ========================================
echo   DNSHealth 健康检测解析 - 开发模式
echo ========================================
echo.
echo  前端开发服务器: http://localhost:5173 (带热重载)
echo  后端 API 服务器: http://localhost:8080
echo.
echo  请在浏览器中访问 http://localhost:5173
echo  API 请求会自动代理到后端 8080 端口
echo ========================================
echo.

echo [1/2] 在新窗口中启动后端服务 (Air 热重载，使用固定 JWT 密钥)...
start "DNS-Backend" cmd /k "echo 后端服务启动中 (修改 Go 文件会自动重启)... & air -- -jwt-secret dev-debug-secret"

echo [2/2] 启动前端开发服务器 (Vite HMR)...
echo 前端修改会自动热重载，无需手动刷新
echo.
cd web
call npm run dev
