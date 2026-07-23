package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/astrastore/astrastore-xion/pkg/auth"
	"github.com/astrastore/astrastore-xion/pkg/files"
	"github.com/astrastore/astrastore-xion/pkg/metadata"
	"github.com/astrastore/astrastore-xion/pkg/monitor"
	"github.com/astrastore/astrastore-xion/pkg/storage"
	"github.com/gorilla/mux"
)

var (
	port          = flag.String("port", "8080", "API网关监听端口")
	configFile    = flag.String("config", "configs/apigateway.yaml", "配置文件路径")
	monitorPort   = flag.String("monitor-port", "9090", "监控端点端口")
	enableMetrics = flag.Bool("enable-metrics", true, "是否启用指标收集")
	jwtSecret     = flag.String("jwt-secret", "your-secret-key", "JWT密钥")
)

func main() {
	flag.Parse()

	// 加载配置文件
	log.Printf("加载配置文件: %s", *configFile)
	// TODO: 加载配置文件

	// 初始化认证和授权
	initAuthSystem()
	initFileService("data/files")

	// 初始化监控
	var mon monitor.Monitor
	if *enableMetrics {
		log.Printf("初始化监控系统...")
		monConfig := monitor.MonitorConfig{
			Endpoint:         ":" + *monitorPort,
			CollectInterval:  10 * time.Second,
			EnablePrometheus: true,
			EnableAlerts:     true,
			DefaultLabels: map[string]string{
				"service": "apigateway",
			},
		}
		mon = monitor.NewPrometheusMonitor(monConfig)

		// 注册基本指标
		mon.RegisterMetric(monitor.MetricDefinition{
			Name:   "apigateway_requests_total",
			Type:   monitor.CounterMetric,
			Help:   "API网关请求总数",
			Labels: []string{"method", "path", "status"},
		})

		mon.RegisterMetric(monitor.MetricDefinition{
			Name:    "apigateway_request_duration_seconds",
			Type:    monitor.HistogramMetric,
			Help:    "API网关请求处理时间",
			Labels:  []string{"method", "path"},
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10},
		})

		mon.RegisterMetric(monitor.MetricDefinition{
			Name:   "apigateway_active_requests",
			Type:   monitor.GaugeMetric,
			Help:   "API网关活跃请求数",
			Labels: []string{},
		})

		// 启动监控服务
		if err := mon.Start(context.Background()); err != nil {
			log.Printf("启动监控服务失败: %v", err)
		} else {
			log.Printf("监控服务启动在端口: %s", *monitorPort)
			defer mon.Stop()
		}
	}

	// 初始化路由
	router := mux.NewRouter()

	// 添加指标中间件
	if mon != nil {
		router.Use(createMetricsMiddleware(mon))
	}

	// 认证路由
	authRouter := router.PathPrefix("/api/v1/auth").Subrouter()
	authRouter.HandleFunc("/login", login).Methods("POST")
	authRouter.HandleFunc("/refresh", refreshToken).Methods("POST")

	// 文件路由
	fileRouter := router.PathPrefix("/api/v1/files").Subrouter()
	fileRouter.HandleFunc("", uploadFile).Methods("POST")
	fileRouter.HandleFunc("", listFiles).Methods("GET")
	fileRouter.HandleFunc("/{id}", downloadFile).Methods("GET")
	fileRouter.HandleFunc("/{id}", deleteFile).Methods("DELETE")
	fileRouter.HandleFunc("/{id}/status", getFileStatus).Methods("GET")

	// 健康检查
	router.HandleFunc("/health", healthCheck).Methods("GET")

	// 设置服务器
	server := &http.Server{
		Addr:         ":" + *port,
		Handler:      router,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// 启动服务器
	go func() {
		log.Printf("API网关启动在端口: %s", *port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("无法启动服务器: %v", err)
		}
	}()

	// 优雅关闭
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("关闭服务器...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("服务器强制关闭: %v", err)
	}

	log.Println("服务器已关闭")
}

// 初始化认证系统
func initAuthSystem() {
	// 创建JWT认证器
	authConfig := auth.AuthConfig{
		SecretKey:     *jwtSecret,
		TokenExpiry:   24 * time.Hour,
		RefreshExpiry: 7 * 24 * time.Hour,
		TokenIssuer:   "astrastore-xion",
		TokenAudience: "astrastore-clients",
	}
	jwtAuth := auth.NewJWTAuthenticator(authConfig)

	// 创建简单授权器
	simpleAuth := auth.NewSimpleAuthorizer()

	// 设置全局变量
	authenticator = jwtAuth
	authorizer = simpleAuth

	// 添加测试用户（实际应从数据库加载）
	adminUser := &auth.User{
		ID:        "admin-001",
		Username:  "admin",
		Email:     "admin@example.com",
		Role:      auth.AdminRole,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	jwtAuth.RegisterUser(adminUser, "admin123")

	operatorUser := &auth.User{
		ID:        "operator-001",
		Username:  "operator",
		Email:     "operator@example.com",
		Role:      auth.OperatorRole,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	jwtAuth.RegisterUser(operatorUser, "operator123")

	normalUser := &auth.User{
		ID:        "user-001",
		Username:  "user",
		Email:     "user@example.com",
		Role:      auth.UserRole,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	jwtAuth.RegisterUser(normalUser, "user123")

	log.Printf("认证系统初始化完成，已添加测试用户")
}

func initFileService(dataPath string) {
	store, err := storage.NewLocalStorageNode(storage.StorageConfig{
		NodeID:   "local-node",
		DataPath: dataPath,
	})
	if err != nil {
		log.Fatalf("初始化文件存储失败: %v", err)
	}

	fileService = files.NewService(
		store,
		metadata.NewInMemoryMetadataService(),
		files.WithNodeID("local-node"),
	)
	log.Printf("文件服务初始化完成，数据目录: %s", dataPath)
}

// 创建指标中间件
func createMetricsMiddleware(mon monitor.Monitor) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 增加活跃请求计数
			mon.AddGauge("apigateway_active_requests", 1, nil)
			defer mon.AddGauge("apigateway_active_requests", -1, nil)

			// 记录开始时间
			start := time.Now()

			// 创建响应记录器
			recorder := &responseRecorder{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// 调用下一个处理器
			next.ServeHTTP(recorder, r)

			// 计算请求处理时间
			duration := time.Since(start).Seconds()

			// 记录请求指标
			labels := map[string]string{
				"method": r.Method,
				"path":   r.URL.Path,
				"status": http.StatusText(recorder.statusCode),
			}
			mon.IncCounter("apigateway_requests_total", 1, labels)
			mon.Observe("apigateway_request_duration_seconds", duration, map[string]string{
				"method": r.Method,
				"path":   r.URL.Path,
			})
		})
	}
}

// 响应记录器
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader 重写WriteHeader方法
func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}
