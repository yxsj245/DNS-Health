package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"dns-health-monitor/internal/api"
	"dns-health-monitor/internal/cache"
	"dns-health-monitor/internal/cname"
	"dns-health-monitor/internal/crypto"
	"dns-health-monitor/internal/database"
	"dns-health-monitor/internal/failover"
	"dns-health-monitor/internal/model"
	"dns-health-monitor/internal/notification"
	"dns-health-monitor/internal/pool"
	"dns-health-monitor/internal/provider"
	"dns-health-monitor/internal/provider/aliyun"
	"dns-health-monitor/internal/scheduler"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// Config 应用配置结构体
type Config struct {
	Server struct {
		Port int    `yaml:"port"`
		Mode string `yaml:"mode"`
	} `yaml:"server"`
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
	JWT struct {
		ExpireHours int `yaml:"expire_hours"`
	} `yaml:"jwt"`
	Encryption struct {
		KeyPath string `yaml:"key_path"`
	} `yaml:"encryption"`
	LogCleaner struct {
		RetentionDays      int `yaml:"retention_days"`
		CleanIntervalHours int `yaml:"clean_interval_hours"`
	} `yaml:"log_cleaner"`
}

// loadConfig 从配置文件加载配置
func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &cfg, nil
}

// loadOrGenerateEncryptionKey 加载或生成 AES-256 加密密钥（32 字节）
// 如果密钥文件不存在，则自动生成并保存
func loadOrGenerateEncryptionKey(keyPath string) ([]byte, error) {
	// 确保密钥文件所在目录存在
	dir := filepath.Dir(keyPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建密钥目录失败: %w", err)
	}

	// 尝试读取已有密钥文件
	key, err := os.ReadFile(keyPath)
	if err == nil {
		// 文件存在，验证密钥长度
		if len(key) != 32 {
			return nil, fmt.Errorf("加密密钥长度无效: 期望 32 字节，实际 %d 字节", len(key))
		}
		log.Printf("已加载加密密钥: %s", keyPath)
		return key, nil
	}

	// 文件不存在，生成新密钥
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("读取密钥文件失败: %w", err)
	}

	key = make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("生成随机密钥失败: %w", err)
	}

	// 保存密钥到文件
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return nil, fmt.Errorf("保存密钥文件失败: %w", err)
	}

	log.Printf("已生成并保存新的加密密钥: %s", keyPath)
	return key, nil
}

// decryptCredentialFields 解密凭证字段，返回明文 map
// 优先从新的 CredentialsEncrypted 字段读取，兼容旧数据格式
func decryptCredentialFields(credential model.Credential, encryptKey []byte) (map[string]string, error) {
	// 新格式：CredentialsEncrypted 存储加密后的 JSON
	if credential.CredentialsEncrypted != "" {
		plainJSON, err := crypto.Decrypt(credential.CredentialsEncrypted, encryptKey)
		if err != nil {
			return nil, fmt.Errorf("解密凭证字段失败: %w", err)
		}
		var fields map[string]string
		if err := json.Unmarshal([]byte(plainJSON), &fields); err != nil {
			return nil, fmt.Errorf("解析凭证 JSON 失败: %w", err)
		}
		return fields, nil
	}

	// 兼容旧格式
	fields := make(map[string]string)
	if credential.AccessKeyIDEncrypted != "" {
		val, err := crypto.Decrypt(credential.AccessKeyIDEncrypted, encryptKey)
		if err != nil {
			return nil, fmt.Errorf("解密 AccessKeyID 失败: %w", err)
		}
		fields["access_key_id"] = val
	}
	if credential.AccessKeySecretEncrypted != "" {
		val, err := crypto.Decrypt(credential.AccessKeySecretEncrypted, encryptKey)
		if err != nil {
			return nil, fmt.Errorf("解密 AccessKeySecret 失败: %w", err)
		}
		fields["access_key_secret"] = val
	}
	return fields, nil
}

// createProviderFactory 创建 DNS 服务商工厂函数
// 接收加密密钥，返回一个根据凭证创建 DNSProvider 的工厂函数
func createProviderFactory(encryptKey []byte) scheduler.ProviderFactory {
	return func(credential model.Credential) (provider.DNSProvider, error) {
		fields, err := decryptCredentialFields(credential, encryptKey)
		if err != nil {
			return nil, err
		}

		// 根据服务商类型创建对应客户端
		switch credential.ProviderType {
		case "aliyun":
			accessKeyID := fields["access_key_id"]
			accessKeySecret := fields["access_key_secret"]
			if accessKeyID == "" || accessKeySecret == "" {
				return nil, fmt.Errorf("阿里云凭证缺少 access_key_id 或 access_key_secret")
			}
			return aliyun.NewAliyunDNSClient(accessKeyID, accessKeySecret), nil
		default:
			return nil, fmt.Errorf("不支持的服务商类型: %s", credential.ProviderType)
		}
	}
}

// 程序启动时间（用于计算运行时长）
var startTime = time.Now()

func main() {
	// 解析命令行参数
	jwtSecretFlag := flag.String("jwt-secret", "", "指定固定的 JWT 签名密钥（开发调试用，不指定则每次启动随机生成）")
	flag.Parse()

	// 1. 加载配置文件
	cfg, err := loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 环境变量 SERVER_PORT 可覆盖配置文件中的端口
	if envPort := os.Getenv("SERVER_PORT"); envPort != "" {
		if p, err := fmt.Sscanf(envPort, "%d", &cfg.Server.Port); err != nil || p != 1 {
			log.Printf("环境变量 SERVER_PORT 值无效: %s，使用配置文件端口", envPort)
		}
	}

	log.Printf("DNSHealth 健康检测解析启动中，监听端口: %d", cfg.Server.Port)

	// 2. 加载或生成加密密钥
	encryptKey, err := loadOrGenerateEncryptionKey(cfg.Encryption.KeyPath)
	if err != nil {
		log.Fatalf("加载加密密钥失败: %v", err)
	}

	// 3. 确保数据库目录存在并初始化数据库
	dbDir := filepath.Dir(cfg.Database.Path)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Fatalf("创建数据库目录失败: %v", err)
	}

	db, err := database.InitDB(cfg.Database.Path)
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	log.Println("数据库初始化完成")

	// 4. 创建已删除记录缓存
	deletedCache := cache.NewDeletedRecordCache(db)

	// 5. 创建 ProviderFactory 和相关依赖
	providerFactory := createProviderFactory(encryptKey)

	// 5.1 创建解析池管理器和探测调度器（需要在调度器之前创建）
	poolManager := pool.NewPoolManager(db)
	poolProber := pool.NewPoolProber(db)

	// 5.2 创建切换类型任务所需的依赖
	cnameResolver := cname.NewCNAMEResolver(db)
	resourceSelector := pool.NewResourceSelector(db)

	// 创建故障转移执行器的 ProviderFactory（类型适配）
	failoverProviderFactory := failover.ProviderFactory(providerFactory)
	failoverExecutor := failover.NewFailoverExecutor(db, failoverProviderFactory, resourceSelector)

	// 5.3 创建通知模块（邮件渠道 + 通知管理器）
	emailChannel := notification.NewEmailChannel()
	notifManager := notification.NewNotificationManager(db, encryptKey, []notification.NotificationChannel{emailChannel})
	log.Println("通知管理器初始化完成")

	// 5.4 创建调度器，注入所有依赖（包括通知管理器）
	sched := scheduler.NewScheduler(db, deletedCache, providerFactory,
		scheduler.WithCNAMEResolver(cnameResolver),
		scheduler.WithFailoverExecutor(failoverExecutor),
		scheduler.WithPoolProber(poolProber),
		scheduler.WithResourceSelector(resourceSelector),
		scheduler.WithNotificationManager(notifManager),
	)

	// 6. 启动调度器（使用可取消的上下文）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.Start(ctx); err != nil {
		log.Fatalf("启动调度器失败: %v", err)
	}
	log.Println("调度器已启动")

	// 7. 生成或使用指定的 JWT 签名密钥
	var jwtSecret []byte
	if *jwtSecretFlag != "" {
		jwtSecret = []byte(*jwtSecretFlag)
		log.Println("使用命令行指定的固定 JWT 签名密钥（仅限开发调试）")
	} else {
		jwtSecret = make([]byte, 32)
		if _, err := rand.Read(jwtSecret); err != nil {
			log.Fatalf("生成 JWT 密钥失败: %v", err)
		}
		log.Println("已生成随机 JWT 签名密钥")
	}
	tokenExpiry := time.Duration(cfg.JWT.ExpireHours) * time.Hour

	authHandler := api.NewAuthHandler(db, jwtSecret, tokenExpiry)
	credHandler := api.NewCredentialHandler(db, encryptKey)
	taskHandler := api.NewTaskHandler(db, sched, poolManager)
	statusHandler := api.NewStatusHandler(db)
	notifHandler := api.NewNotificationHandler(db, encryptKey, notifManager)

	// 启动解析池探测调度器（恢复所有解析池的探测活动）
	if err := poolProber.Start(ctx); err != nil {
		log.Printf("启动解析池探测调度器失败: %v", err)
	} else {
		log.Println("解析池探测调度器已启动")
	}

	// 启动日志清理器（定期清理过期的探测结果和操作日志）
	cleanerConfig := scheduler.DefaultCleanerConfig()
	if cfg.LogCleaner.RetentionDays > 0 {
		cleanerConfig.RetentionDays = cfg.LogCleaner.RetentionDays
	}
	if cfg.LogCleaner.CleanIntervalHours > 0 {
		cleanerConfig.CleanInterval = time.Duration(cfg.LogCleaner.CleanIntervalHours) * time.Hour
	}
	cleaner := scheduler.NewCleaner(db, cleanerConfig)
	cleaner.Start(ctx)

	poolHandler := api.NewPoolHandler(poolManager, poolProber)

	// 8. 设置 Gin 模式并创建路由
	gin.SetMode(cfg.Server.Mode)
	useFixedSecret := *jwtSecretFlag != ""
	router := api.SetupRouter(authHandler, credHandler, taskHandler, statusHandler, poolHandler, notifHandler, jwtSecret, useFixedSecret, startTime)

	// 9. 嵌入前端静态文件服务
	// 如果 web/dist 目录存在，提供静态文件服务
	if _, err := os.Stat("web/dist"); err == nil {
		router.Static("/assets", "web/dist/assets")
		router.StaticFile("/favicon.ico", "web/dist/favicon.ico")
		router.StaticFile("/logo.png", "web/dist/logo.png")

		// 对于非 API 路由，返回 index.html（支持 SPA 前端路由）
		router.NoRoute(func(c *gin.Context) {
			// 如果请求路径以 /api 开头，返回 404
			if len(c.Request.URL.Path) >= 4 && c.Request.URL.Path[:4] == "/api" {
				c.JSON(http.StatusNotFound, gin.H{"error": "接口不存在"})
				return
			}
			c.File("web/dist/index.html")
		})
		log.Println("前端静态文件服务已启用: web/dist/")
	} else {
		log.Println("未找到前端构建文件 (web/dist/)，跳过静态文件服务")
	}

	// 10. 创建 HTTP 服务器
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: router,
	}

	// 11. 在 goroutine 中启动 HTTP 服务
	go func() {
		log.Printf("HTTP 服务已启动，监听端口: %d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP 服务启动失败: %v", err)
		}
	}()

	// 12. 优雅关闭：监听系统信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("收到信号 %v，开始优雅关闭...", sig)

	// 停止调度器
	sched.Stop()
	log.Println("调度器已停止")

	// 停止解析池探测调度器
	poolProber.Stop()
	log.Println("解析池探测调度器已停止")

	// 取消上下文
	cancel()

	// 关闭 HTTP 服务（给 5 秒超时时间）
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP 服务关闭异常: %v", err)
	} else {
		log.Println("HTTP 服务已关闭")
	}

	log.Println("DNSHealth 健康检测解析已安全退出")
}
