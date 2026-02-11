@echo off
chcp 65001 >nul
echo ========================================
echo   DNSHealth 健康检测解析 - 仅编译
echo ========================================

echo [1/2] 编译前端...
cd web
call npm run build
if %errorlevel% neq 0 (
    echo 前端编译失败！
    cd ..
    pause
    exit /b 1
)
cd ..

echo [2/2] 编译后端...
go build -o dns-health-monitor.exe .
if %errorlevel% neq 0 (
    echo 后端编译失败！
    pause
    exit /b 1
)

echo ========================================
echo 编译完成！生成文件: dns-health-monitor.exe
echo 运行命令: .\dns-health-monitor.exe
echo ========================================
pause
