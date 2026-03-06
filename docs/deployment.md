# DNSHealth 部署指南

本文档详细介绍了 DNSHealth 健康检测解析系统的各种部署方式，涵盖 **Docker 部署**、**Linux 部署** 和 **Windows 部署** 三种场景。

---

## 目录

- [环境要求](#环境要求)
- [Docker 部署（推荐）](#docker-部署推荐)
  - [使用 Docker Compose 部署](#使用-docker-compose-部署)
  - [使用 Docker CLI 部署](#使用-docker-cli-部署)
  - [Docker 构建自定义镜像](#docker-构建自定义镜像)
  - [Docker 多架构支持](#docker-多架构支持)
- [Linux 部署](#linux-部署)
  - [使用预编译二进制文件](#linux-使用预编译二进制文件)
  - [从源码编译](#linux-从源码编译)
  - [配置 Systemd 服务](#配置-systemd-服务)
  - [使用 Nginx 反向代理](#使用-nginx-反向代理)
- [Windows 部署](#windows-部署)
  - [使用预编译二进制文件](#windows-使用预编译二进制文件)
  - [从源码编译](#windows-从源码编译)
  - [注册为 Windows 服务](#注册为-windows-服务)
- [配置文件说明](#配置文件说明)
- [数据持久化与备份](#数据持久化与备份)
- [常见问题排查](#常见问题排查)

---

## 环境要求

| 部署方式 | 要求 |
|---------|------|
| Docker | Docker Engine 20.10+ / Docker Compose V2+ |
| Linux  | x86_64 或 ARM64 架构，glibc 2.17+ |
| Windows | Windows 10/11 或 Windows Server 2016+，x86_64 架构 |
| 源码编译 | Go 1.24+，Node.js 20+，npm 9+ |

> [!IMPORTANT]
> **ICMP 探测**（Ping）需要特殊权限：
> - Docker：需要 `NET_RAW` 能力
> - Linux：需要以 `root` 用户运行，或通过 `setcap` 授权
> - Windows：需要以**管理员身份**运行

---

## Docker 部署（推荐）

Docker 是最简单的部署方式，项目提供了预构建的多架构镜像，支持 `linux/amd64` 和 `linux/arm64`。

### 使用 Docker Compose 部署

**1. 创建项目目录**

```bash
mkdir -p /opt/dns-health && cd /opt/dns-health
```

**2. 创建 `docker-compose.yml`**

```yaml
services:
  dns-health-monitor:
    image: xiaozhu674/dns-health:latest
    container_name: dns-health-monitor
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      # 持久化数据库和加密密钥
      - ./data:/app/data
    environment:
      - TZ=Asia/Shanghai
      - SERVER_PORT=8080
    # ICMP 探测需要 NET_RAW 权限
    cap_add:
      - NET_RAW
```

**3. 启动服务**

```bash
docker compose up -d
```

**4. 查看日志**

```bash
docker compose logs -f
```

**5. 访问 Web 界面**

打开浏览器访问 `http://<服务器IP>:8080`，首次访问会进入注册页面，创建管理员账号。

**6. 常用管理命令**

```bash
# 停止服务
docker compose down

# 重启服务
docker compose restart

# 更新到最新版本
docker compose pull
docker compose up -d

# 查看实时日志
docker compose logs -f --tail 100
```

### 使用 Docker CLI 部署

如果不想使用 Docker Compose，也可以直接使用 Docker 命令：

```bash
# 创建数据目录
mkdir -p /opt/dns-health/data

# 运行容器
docker run -d \
  --name dns-health-monitor \
  --restart unless-stopped \
  -p 8080:8080 \
  -v /opt/dns-health/data:/app/data \
  -e TZ=Asia/Shanghai \
  -e SERVER_PORT=8080 \
  --cap-add NET_RAW \
  xiaozhu674/dns-health:latest
```

### Docker 构建自定义镜像

如果需要自定义镜像（例如修改了源码），可以从源码构建：

```bash
# 克隆项目
git clone https://github.com/yxsj245/DNS-Health.git
cd DNS-Health

# 构建镜像
docker build -t dns-health:custom .

# 使用自定义镜像运行
docker run -d \
  --name dns-health-monitor \
  --restart unless-stopped \
  -p 8080:8080 \
  -v ./data:/app/data \
  -e TZ=Asia/Shanghai \
  --cap-add NET_RAW \
  dns-health:custom
```

### Docker 多架构支持

项目的 CI/CD 流水线会自动构建 `linux/amd64` 和 `linux/arm64` 双架构镜像。Docker 会根据宿主机架构自动拉取对应的镜像版本，无需额外配置。

如需手动构建多架构镜像：

```bash
# 创建并使用 buildx 构建器
docker buildx create --name multiarch --use

# 构建并推送多架构镜像
docker buildx build --platform linux/amd64,linux/arm64 \
  -t your-registry/dns-health:latest \
  --push .
```

---

## Linux 部署

### Linux 使用预编译二进制文件

**1. 下载发布包**

前往 [Releases 页面](https://github.com/yxsj245/DNS-Health/releases) 下载对应架构的压缩包：

- `dns-health-linux-arm64.tar.gz`（ARM64 架构）

> [!NOTE]
> 如果需要 x86_64 版本，请从源码编译或使用 Docker 部署。

**2. 解压并安装**

```bash
# 解压
tar -xzf dns-health-linux-arm64.tar.gz

# 移动到安装目录
sudo mv dns-health-linux-arm64 /opt/dns-health

# 进入目录
cd /opt/dns-health

# 赋予执行权限
chmod +x dns-health
```

**3. 修改配置（可选）**

编辑 `config.yaml` 文件，根据需要修改端口、数据库路径等配置：

```bash
vim config.yaml
```

**4. 运行**

```bash
# 前台运行（测试用）
sudo ./dns-health

# 后台运行
sudo nohup ./dns-health > /var/log/dns-health.log 2>&1 &
```

> [!TIP]
> 建议使用 Systemd 服务管理，参见下文 [配置 Systemd 服务](#配置-systemd-服务)。

**5. 授权 ICMP（非 root 用户运行时）**

如果不想以 root 用户运行，可以通过 `setcap` 授予 ICMP 权限：

```bash
sudo setcap cap_net_raw+ep /opt/dns-health/dns-health
```

### Linux 从源码编译

**1. 安装依赖**

```bash
# Ubuntu / Debian
sudo apt update
sudo apt install -y golang nodejs npm git

# CentOS / RHEL
sudo yum install -y golang nodejs npm git

# 或使用官方安装方式
# Go: https://go.dev/dl/
# Node.js: https://nodejs.org/
```

确保版本满足要求：

```bash
go version    # 需要 1.24+
node --version  # 需要 20+
npm --version   # 需要 9+
```

**2. 克隆项目并编译**

```bash
# 克隆项目
git clone https://github.com/yxsj245/DNS-Health.git
cd DNS-Health

# 编译前端
cd web
npm ci
npm run build
cd ..

# 编译后端
CGO_ENABLED=0 go build -ldflags="-s -w" -o dns-health .
```

**3. 部署文件**

```bash
# 创建安装目录
sudo mkdir -p /opt/dns-health

# 复制必要文件
sudo cp dns-health /opt/dns-health/
sudo cp config.yaml /opt/dns-health/
sudo cp -r web/dist /opt/dns-health/web/dist

# 创建数据目录
sudo mkdir -p /opt/dns-health/data

# 设置权限
sudo chmod +x /opt/dns-health/dns-health
```

### 配置 Systemd 服务

推荐将 DNSHealth 注册为 Systemd 服务，实现开机自启和自动重启。

**1. 创建服务文件**

```bash
sudo tee /etc/systemd/system/dns-health.service > /dev/null << 'EOF'
[Unit]
Description=DNSHealth 健康检测解析系统
Documentation=https://github.com/yxsj245/DNS-Health
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/dns-health
ExecStart=/opt/dns-health/dns-health
Restart=on-failure
RestartSec=10
LimitNOFILE=65536

# 环境变量
Environment=TZ=Asia/Shanghai
Environment=SERVER_PORT=8080

# 日志输出
StandardOutput=journal
StandardError=journal
SyslogIdentifier=dns-health

[Install]
WantedBy=multi-user.target
EOF
```

**2. 启用并启动服务**

```bash
# 重新加载 systemd
sudo systemctl daemon-reload

# 启用开机自启
sudo systemctl enable dns-health

# 启动服务
sudo systemctl start dns-health

# 查看状态
sudo systemctl status dns-health

# 查看日志
sudo journalctl -u dns-health -f
```

**3. 常用管理命令**

```bash
# 停止服务
sudo systemctl stop dns-health

# 重启服务
sudo systemctl restart dns-health

# 禁用开机自启
sudo systemctl disable dns-health
```

### 使用 Nginx 反向代理

如果需要通过域名或 HTTPS 访问，可以配置 Nginx 反向代理：

```nginx
server {
    listen 80;
    server_name dns-health.example.com;

    # 如需 HTTPS，取消以下注释并配置 SSL 证书
    # listen 443 ssl;
    # ssl_certificate /etc/nginx/ssl/cert.pem;
    # ssl_certificate_key /etc/nginx/ssl/key.pem;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket 支持（如需要）
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

```bash
# 测试 Nginx 配置
sudo nginx -t

# 重新加载 Nginx
sudo systemctl reload nginx
```

---

## Windows 部署

### Windows 使用预编译二进制文件

**1. 下载发布包**

前往 [Releases 页面](https://github.com/yxsj245/DNS-Health/releases) 下载 Windows 版本：

- `dns-health-windows-amd64.zip`

**2. 解压**

将压缩包解压到目标目录，例如 `C:\dns-health\`。

解压后目录结构如下：

```
C:\dns-health\
├── dns-health.exe          # 主程序
├── config.yaml             # 配置文件
├── web/
│   └── dist/               # 前端静态文件
└── data/                   # 数据目录（首次运行自动创建）
```

**3. 修改配置（可选）**

用文本编辑器打开 `config.yaml`，根据需要修改配置。

**4. 运行**

> [!WARNING]
> ICMP 探测（Ping）功能需要**以管理员身份运行**，否则 Ping 探测将无法正常工作。

- **方式一**：右键 `dns-health.exe` → **以管理员身份运行**
- **方式二**：以管理员身份打开 CMD 或 PowerShell，然后执行：

```powershell
cd C:\dns-health
.\dns-health.exe
```

**5. 访问 Web 界面**

打开浏览器访问 `http://localhost:8080`，首次访问会进入注册页面。

### Windows 从源码编译

**1. 安装依赖**

- 安装 [Go 1.24+](https://go.dev/dl/)
- 安装 [Node.js 20+](https://nodejs.org/)
- 安装 [Git](https://git-scm.com/)

**2. 克隆并编译**

打开 PowerShell 或 CMD：

```powershell
# 克隆项目
git clone https://github.com/yxsj245/DNS-Health.git
cd DNS-Health

# 编译前端
cd web
npm ci
npm run build
cd ..

# 编译后端
go build -o dns-health.exe .
```

或者直接使用项目自带的编译脚本：

```powershell
# 仅编译
.\build.bat

# 编译并运行
.\run.bat
```

### 注册为 Windows 服务

使用 [NSSM（Non-Sucking Service Manager）](https://nssm.cc/) 将 DNSHealth 注册为 Windows 服务，实现开机自启。

**1. 下载 NSSM**

从 [nssm.cc](https://nssm.cc/download) 下载 NSSM，解压后将 `nssm.exe` 放到 `C:\dns-health\` 目录或系统 PATH 中。

**2. 安装服务**

以管理员身份打开 PowerShell：

```powershell
# 安装服务
nssm install DNSHealth "C:\dns-health\dns-health.exe"

# 设置工作目录
nssm set DNSHealth AppDirectory "C:\dns-health"

# 设置启动类型为自动
nssm set DNSHealth Start SERVICE_AUTO_START

# 配置日志输出
nssm set DNSHealth AppStdout "C:\dns-health\logs\stdout.log"
nssm set DNSHealth AppStderr "C:\dns-health\logs\stderr.log"

# 启动服务
nssm start DNSHealth
```

**3. 常用管理命令**

```powershell
# 查看服务状态
nssm status DNSHealth

# 停止服务
nssm stop DNSHealth

# 重启服务
nssm restart DNSHealth

# 卸载服务
nssm remove DNSHealth confirm
```

> [!TIP]
> 也可以使用 `nssm edit DNSHealth` 命令打开图形化编辑界面来管理服务配置。

---

## 配置文件说明

项目使用 `config.yaml` 作为配置文件，以下是完整的配置项说明：

```yaml
# DNSHealth 健康检测解析配置文件

# 服务器配置
server:
  # 监听端口（可通过环境变量 SERVER_PORT 覆盖）
  port: 8080
  # 运行模式：debug（调试）/ release（生产）
  mode: release

# 数据库配置
database:
  # SQLite 数据库文件路径（相对于工作目录）
  path: data/dns-monitor.db

# JWT 认证配置
jwt:
  # Token 有效期（小时）
  expire_hours: 24

# 加密配置
encryption:
  # AES-256 加密密钥文件路径
  # 首次运行时自动生成，请妥善保管
  key_path: data/encryption.key

# 日志清理配置
log_cleaner:
  # 数据保留天数（默认 30 天）
  retention_days: 30
  # 清理任务执行间隔（小时，默认 24 小时）
  clean_interval_hours: 24
```

### 环境变量

以下环境变量可覆盖配置文件中的设置：

| 环境变量 | 说明 | 默认值 |
|---------|------|-------|
| `SERVER_PORT` | 服务监听端口 | `8080` |
| `TZ` | 时区设置 | `Asia/Shanghai` |

---

## 数据持久化与备份

### 重要数据文件

DNSHealth 的所有持久化数据存储在 `data/` 目录下：

| 文件 | 说明 | 重要性 |
|------|------|--------|
| `data/dns-monitor.db` | SQLite 数据库，包含所有任务、凭证、探测记录等 | ⚠️ 关键 |
| `data/encryption.key` | AES-256 加密密钥，用于加密/解密云服务商凭证 | ⚠️ 关键 |

> [!CAUTION]
> `encryption.key` 文件**极其重要**！如果丢失此文件，所有已保存的云服务商凭证将**无法解密**，需要重新配置。请务必妥善备份。

### 备份建议

```bash
# Linux 备份命令
tar -czf dns-health-backup-$(date +%Y%m%d).tar.gz /opt/dns-health/data/

# Docker 环境备份
docker compose stop
tar -czf dns-health-backup-$(date +%Y%m%d).tar.gz ./data/
docker compose start
```

```powershell
# Windows 备份命令 (PowerShell)
$date = Get-Date -Format "yyyyMMdd"
Compress-Archive -Path "C:\dns-health\data\*" -DestinationPath "C:\backup\dns-health-backup-$date.zip"
```

### 迁移步骤

1. 停止旧服务
2. 复制整个 `data/` 目录到新环境
3. 在新环境中部署并启动服务
4. 验证数据是否正常加载

---

## 常见问题排查

### 1. 端口被占用

```bash
# Linux 查看端口占用
sudo lsof -i :8080
sudo ss -tlnp | grep 8080

# Windows 查看端口占用
netstat -ano | findstr :8080
```

修改 `config.yaml` 中的 `port` 或设置环境变量 `SERVER_PORT` 更换端口。

### 2. ICMP 探测权限问题

**Linux：**
```bash
# 方式一：以 root 运行
sudo ./dns-health

# 方式二：授予 CAP_NET_RAW
sudo setcap cap_net_raw+ep ./dns-health
```

**Docker：**
确保 `docker-compose.yml` 或 `docker run` 命令中包含 `--cap-add NET_RAW`。

**Windows：**
以管理员身份运行程序。

### 3. Docker 容器无法启动

```bash
# 查看容器日志
docker logs dns-health-monitor

# 检查数据目录权限
ls -la ./data/
```

### 4. 数据库损坏恢复

如果数据库文件损坏，可以尝试：

```bash
# 备份损坏的数据库
cp data/dns-monitor.db data/dns-monitor.db.bak

# 删除数据库（将丢失所有数据，保留加密密钥）
rm data/dns-monitor.db

# 重新启动服务，会自动创建新数据库
```

### 5. 防火墙配置

```bash
# Linux (firewalld)
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload

# Linux (ufw)
sudo ufw allow 8080/tcp

# Windows (PowerShell，管理员)
New-NetFirewallRule -DisplayName "DNSHealth" -Direction Inbound -Protocol TCP -LocalPort 8080 -Action Allow
```
