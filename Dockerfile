# ==================== 前端构建阶段 ====================
FROM node:20-alpine AS frontend-builder

WORKDIR /app/web

# 先复制依赖文件，利用 Docker 缓存
COPY web/package.json web/package-lock.json ./
RUN npm ci

# 复制前端源码并构建
COPY web/ ./
RUN npm run build

# ==================== 后端构建阶段 ====================
FROM golang:1.24-alpine AS backend-builder

# 安装 CGO 依赖（SQLite 需要）
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# 先复制依赖文件，利用 Docker 缓存
COPY go.mod go.sum ./
RUN go mod download

# 复制后端源码并编译
COPY main.go ./
COPY internal/ ./internal/

# 启用 CGO（SQLite 需要），静态链接
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o dns-health-monitor .

# ==================== 运行阶段 ====================
FROM alpine:3.20

# 安装运行时依赖
# ca-certificates: HTTPS 请求需要
# tzdata: 时区支持
RUN apk add --no-cache ca-certificates tzdata

# 设置时区为亚洲/上海
ENV TZ=Asia/Shanghai

WORKDIR /app

# 从构建阶段复制二进制文件
COPY --from=backend-builder /app/dns-health-monitor .

# 从前端构建阶段复制静态文件
COPY --from=frontend-builder /app/web/dist ./web/dist/

# 复制默认配置文件
COPY config.yaml ./

# 创建数据目录
RUN mkdir -p /app/data

# 数据目录挂载点（持久化数据库和密钥）
VOLUME ["/app/data"]

EXPOSE 8080

ENTRYPOINT ["./dns-health-monitor"]
