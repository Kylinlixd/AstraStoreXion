package monitor

import (
	"context"
	"net/http"
	"time"
)

// MetricType 指标类型
type MetricType string

const (
	// 计数器类型，只增不减
	CounterMetric MetricType = "counter"
	// 仪表盘类型，可增可减
	GaugeMetric MetricType = "gauge"
	// 直方图类型，用于分布统计
	HistogramMetric MetricType = "histogram"
	// 摘要类型，用于分位数统计
	SummaryMetric MetricType = "summary"
)

// MetricDefinition 指标定义
type MetricDefinition struct {
	Name        string              // 指标名称
	Type        MetricType          // 指标类型
	Help        string              // 帮助信息
	Labels      []string            // 标签列表
	Buckets     []float64           // 直方图桶（仅用于直方图类型）
	Objectives  map[float64]float64 // 摘要目标（仅用于摘要类型）
	Constlabels map[string]string   // 常量标签
}

// AlertLevel 告警级别
type AlertLevel string

const (
	// 信息级别，通知性质
	InfoAlert AlertLevel = "info"
	// 警告级别，需要关注
	WarnAlert AlertLevel = "warning"
	// 错误级别，需要处理
	ErrorAlert AlertLevel = "error"
	// 严重级别，需要立即处理
	CriticalAlert AlertLevel = "critical"
)

// AlertRule 告警规则
type AlertRule struct {
	Name        string            // 规则名称
	Query       string            // 查询表达式
	Duration    time.Duration     // 持续时间
	Labels      map[string]string // 标签
	Annotations map[string]string // 注释
	Level       AlertLevel        // 告警级别
}

// AlertNotifier 告警通知器
type AlertNotifier interface {
	// 发送告警
	Send(ctx context.Context, alert Alert) error
	// 获取通知器名称
	Name() string
}

// Alert 告警信息
type Alert struct {
	Name         string            // 告警名称
	Level        AlertLevel        // 告警级别
	Message      string            // 告警消息
	Labels       map[string]string // 标签
	Annotations  map[string]string // 注释
	StartsAt     time.Time         // 开始时间
	EndsAt       time.Time         // 结束时间（可选）
	GeneratorURL string            // 生成器URL（可选）
}

// Monitor 监控接口
type Monitor interface {
	// 注册指标
	RegisterMetric(def MetricDefinition) error

	// 更新计数器
	IncCounter(name string, value float64, labels map[string]string) error

	// 更新仪表盘
	SetGauge(name string, value float64, labels map[string]string) error

	// 按增量更新仪表盘
	AddGauge(name string, delta float64, labels map[string]string) error

	// 观察直方图或摘要
	Observe(name string, value float64, labels map[string]string) error

	// 添加告警规则
	AddAlertRule(rule AlertRule) error

	// 删除告警规则
	RemoveAlertRule(name string) error

	// 注册告警通知器
	RegisterNotifier(notifier AlertNotifier) error

	// 启动监控服务
	Start(ctx context.Context) error

	// 停止监控服务
	Stop() error
}

// MonitorConfig 监控配置
type MonitorConfig struct {
	Endpoint         string            // 监控端点
	CollectInterval  time.Duration     // 收集间隔
	EnablePrometheus bool              // 是否启用Prometheus
	EnableJaeger     bool              // 是否启用Jaeger
	EnableAlerts     bool              // 是否启用告警
	AlertManagerURL  string            // AlertManager URL
	DefaultLabels    map[string]string // 默认标签
}

// MonitorServer 监控服务器
type MonitorServer struct {
	config    MonitorConfig
	server    *http.Server
	notifiers []AlertNotifier
	running   bool
}

// NewMonitorServer 创建监控服务器
func NewMonitorServer(config MonitorConfig) *MonitorServer {
	return &MonitorServer{
		config:    config,
		notifiers: make([]AlertNotifier, 0),
	}
}

// RegisterNotifier 注册告警通知器
func (s *MonitorServer) RegisterNotifier(notifier AlertNotifier) error {
	s.notifiers = append(s.notifiers, notifier)
	return nil
}

// Start 启动监控服务器
func (s *MonitorServer) Start(ctx context.Context) error {
	if s.running {
		return nil
	}

	// 创建HTTP服务器
	mux := http.NewServeMux()

	// 注册指标端点
	if s.config.EnablePrometheus {
		mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
			// 导出Prometheus指标
			w.Write([]byte("# Prometheus metrics will be exported here\n"))
		})
	}

	// 注册健康检查端点
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// 创建HTTP服务器
	s.server = &http.Server{
		Addr:    s.config.Endpoint,
		Handler: mux,
	}

	// 启动HTTP服务器
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// 记录错误
		}
	}()

	s.running = true
	return nil
}

// Stop 停止监控服务器
func (s *MonitorServer) Stop() error {
	if !s.running {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return err
	}

	s.running = false
	return nil
}
