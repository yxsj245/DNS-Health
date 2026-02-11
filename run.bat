@echo off
chcp 65001 >nul
echo ========================================
echo   DNSHealth 健康检测解析 - 编译并运行
echo ========================================

echo [1/3] 编译前端...
cd web
call npm run build
if %errorlevel% neq 0 (
    echo 前端编译失败！
    cd ..
    pause
    exit /b 1
)
cd ..

echo [2/3] 编译后端...
go build -o dns-health-monitor.exe .
if %errorlevel% neq 0 (
    echo 后端编译失败！
    pause
    exit /b 1
)

echo [3/3] 启动服务...
echo 访问地址: http://localhost:8080
echo 默认账号: admin / admin123
echo 按 Ctrl+C 停止服务
echo ========================================
dns-health-monitor.exe
