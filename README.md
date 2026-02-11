# DNS 健康检测自动暂停/恢复系统

通过周期性健康探测监控域名解析记录中各 IP 的可用性，当 IP 不可达时自动暂停或删除对应 DNS 记录，当 IP 恢复时自动恢复或重新添加记录。

## 功能特性

- 多协议健康探测：ICMP、TCP、UDP、HTTP、HTTPS
- 自动暂停/恢复 DNS 记录（支持阿里云 DNS）
- 最后一条记录保护，避免域名完全失效
- 已删除记录持久化缓存，确保恢复后能重新添加
- Web 控制台管理探测任务、凭证、查看历史
- JWT 认证，凭证 AES-GCM 加密存储
- SQLite 嵌入式数据库，无需额外部署

## 快速开始

### 环境要求

- Go 1.21+
- Node.js 18+（构建前端）

### 编译运行

```bash
# 方式一：使用脚本（Windows）
run.bat        # 编译并运行
build.bat      # 仅编译

# 方式二：手动编译
cd web && npm install && npm run build && cd ..
go build -o dns-health-monitor.exe .
.\dns-health-monitor.exe
```

### 访问系统

启动后访问 `http://localhost:8080`

默认管理员账号：
- 用户名：`admin`
- 密码：`admin123`

## 配置说明

编辑 `config.yaml`：

```yaml
server:
  port: 8080          # 监听端口
  mode: release       # debug / release

database:
  path: data/dns-monitor.db

jwt:
  secret: change-me-to-a-random-secret-key  # 生产环境请修改
  expire_hours: 24

encryption:
  key_path: data/encryption.key  # 自动生成

admin:
  username: admin
  password: admin123   # 仅首次启动时创建
```

## 项目结构

```
├── main.go                    # 程序入口
├── config.yaml                # 配置文件
├── internal/
│   ├── model/                 # 数据模型
│   ├── database/              # 数据库初始化
│   ├── prober/                # 健康探测模块（ICMP/TCP/UDP/HTTP/HTTPS）
│   ├── provider/              # 云服务商对接
│   │   └── aliyun/            # 阿里云 DNS 实现
│   ├── scheduler/             # 监控调度器
│   ├── cache/                 # 已删除记录缓存
│   ├── crypto/                # AES-GCM 加密
│   └── api/                   # Web API 层（Gin）
├── web/                       # Vue 3 前端
│   └── src/views/             # 页面组件
└── data/                      # 运行时数据（数据库、密钥）
```

## 技术栈

| 组件 | 技术 |
|------|------|
| 后端 | Go + Gin + GORM |
| 数据库 | SQLite（纯 Go 驱动） |
| 认证 | JWT |
| 前端 | Vue 3 + Vite + Element Plus |
| 加密 | AES-256-GCM |

## 许可证

MIT
