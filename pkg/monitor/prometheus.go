package monitor

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// PrometheusMonitor Prometheus监控实现
type PrometheusMonitor struct {
	registry   *prometheus.Registry
	metrics    map[string]interface{}
	alertRules map[string]AlertRule
	notifiers  []AlertNotifier
	mutex      sync.RWMutex
	config     MonitorConfig
	running    bool
	httpServer *http.Server
}

// NewPrometheusMonitor 创建Prometheus监控
func NewPrometheusMonitor(config MonitorConfig) *PrometheusMonitor {
	return &PrometheusMonitor{
		registry:   prometheus.NewRegistry(),
		metrics:    make(map[string]interface{}),
		alertRules: make(map[string]AlertRule),
		notifiers:  make([]AlertNotifier, 0),
		config:     config,
	}
}

// RegisterMetric 注册指标
func (m *PrometheusMonitor) RegisterMetric(def MetricDefinition) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// 检查指标是否已存在
	if _, exists := m.metrics[def.Name]; exists {
		return fmt.Errorf("指标 %s 已存在", def.Name)
	}

	// 创建标签
	labelNames := make([]string, len(def.Labels))
	copy(labelNames, def.Labels)

	// 根据指标类型创建不同的指标
	var metric interface{}
	var err error

	switch def.Type {
	case CounterMetric:
		opts := prometheus.CounterOpts{
			Name:        def.Name,
			Help:        def.Help,
			ConstLabels: def.Constlabels,
		}
		if len(labelNames) > 0 {
			metric = prometheus.NewCounterVec(opts, labelNames)
		} else {
			metric = prometheus.NewCounter(opts)
		}

	case GaugeMetric:
		opts := prometheus.GaugeOpts{
			Name:        def.Name,
			Help:        def.Help,
			ConstLabels: def.Constlabels,
		}
		if len(labelNames) > 0 {
			metric = prometheus.NewGaugeVec(opts, labelNames)
		} else {
			metric = prometheus.NewGauge(opts)
		}

	case HistogramMetric:
		opts := prometheus.HistogramOpts{
			Name:        def.Name,
			Help:        def.Help,
			ConstLabels: def.Constlabels,
			Buckets:     def.Buckets,
		}
		if len(labelNames) > 0 {
			metric = prometheus.NewHistogramVec(opts, labelNames)
		} else {
			metric = prometheus.NewHistogram(opts)
		}

	case SummaryMetric:
		opts := prometheus.SummaryOpts{
			Name:        def.Name,
			Help:        def.Help,
			ConstLabels: def.Constlabels,
			Objectives:  def.Objectives,
		}
		if len(labelNames) > 0 {
			metric = prometheus.NewSummaryVec(opts, labelNames)
		} else {
			metric = prometheus.NewSummary(opts)
		}

	default:
		return fmt.Errorf("不支持的指标类型: %s", def.Type)
	}

	// 注册到Prometheus
	if err = m.registry.Register(metric.(prometheus.Collector)); err != nil {
		return fmt.Errorf("注册指标失败: %v", err)
	}

	// 保存指标
	m.metrics[def.Name] = metric
	return nil
}

// IncCounter 增加计数器
func (m *PrometheusMonitor) IncCounter(name string, value float64, labels map[string]string) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	metric, exists := m.metrics[name]
	if !exists {
		return fmt.Errorf("指标 %s 不存在", name)
	}

	// 根据是否有标签调用不同的方法
	if counter, ok := metric.(prometheus.Counter); ok {
		counter.Add(value)
		return nil
	}

	if counterVec, ok := metric.(*prometheus.CounterVec); ok {
		counter, err := counterVec.GetMetricWith(labels)
		if err != nil {
			return fmt.Errorf("获取指标失败: %v", err)
		}
		counter.Add(value)
		return nil
	}

	return fmt.Errorf("指标 %s 不是计数器类型", name)
}

// SetGauge 设置仪表盘值
func (m *PrometheusMonitor) SetGauge(name string, value float64, labels map[string]string) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	metric, exists := m.metrics[name]
	if !exists {
		return fmt.Errorf("指标 %s 不存在", name)
	}

	// 根据是否有标签调用不同的方法
	if gauge, ok := metric.(prometheus.Gauge); ok {
		gauge.Set(value)
		return nil
	}

	if gaugeVec, ok := metric.(*prometheus.GaugeVec); ok {
		gauge, err := gaugeVec.GetMetricWith(labels)
		if err != nil {
			return fmt.Errorf("获取指标失败: %v", err)
		}
		gauge.Set(value)
		return nil
	}

	return fmt.Errorf("指标 %s 不是仪表盘类型", name)
}

// AddGauge 按增量更新仪表盘值
func (m *PrometheusMonitor) AddGauge(name string, delta float64, labels map[string]string) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	metric, exists := m.metrics[name]
	if !exists {
		return fmt.Errorf("指标 %s 不存在", name)
	}

	if gauge, ok := metric.(prometheus.Gauge); ok {
		gauge.Add(delta)
		return nil
	}

	if gaugeVec, ok := metric.(*prometheus.GaugeVec); ok {
		gauge, err := gaugeVec.GetMetricWith(labels)
		if err != nil {
			return fmt.Errorf("获取指标失败: %v", err)
		}
		gauge.Add(delta)
		return nil
	}

	return fmt.Errorf("指标 %s 不是仪表盘类型", name)
}

// Observe 观察值
func (m *PrometheusMonitor) Observe(name string, value float64, labels map[string]string) error {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	metric, exists := m.metrics[name]
	if !exists {
		return fmt.Errorf("指标 %s 不存在", name)
	}

	// 处理直方图
	if histogram, ok := metric.(prometheus.Histogram); ok {
		histogram.Observe(value)
		return nil
	}

	if histogramVec, ok := metric.(*prometheus.HistogramVec); ok {
		histogram, err := histogramVec.GetMetricWith(labels)
		if err != nil {
			return fmt.Errorf("获取指标失败: %v", err)
		}
		histogram.Observe(value)
		return nil
	}

	// 处理摘要
	if summary, ok := metric.(prometheus.Summary); ok {
		summary.Observe(value)
		return nil
	}

	if summaryVec, ok := metric.(*prometheus.SummaryVec); ok {
		summary, err := summaryVec.GetMetricWith(labels)
		if err != nil {
			return fmt.Errorf("获取指标失败: %v", err)
		}
		summary.Observe(value)
		return nil
	}

	return fmt.Errorf("指标 %s 不是直方图或摘要类型", name)
}

// AddAlertRule 添加告警规则
func (m *PrometheusMonitor) AddAlertRule(rule AlertRule) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.alertRules[rule.Name]; exists {
		return fmt.Errorf("告警规则 %s 已存在", rule.Name)
	}

	m.alertRules[rule.Name] = rule
	return nil
}

// RemoveAlertRule 删除告警规则
func (m *PrometheusMonitor) RemoveAlertRule(name string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.alertRules[name]; !exists {
		return fmt.Errorf("告警规则 %s 不存在", name)
	}

	delete(m.alertRules, name)
	return nil
}

// RegisterNotifier 注册告警通知器
func (m *PrometheusMonitor) RegisterNotifier(notifier AlertNotifier) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.notifiers = append(m.notifiers, notifier)
	return nil
}

// Start 启动监控服务
func (m *PrometheusMonitor) Start(ctx context.Context) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.running {
		return nil
	}

	// 创建HTTP服务器
	mux := http.NewServeMux()

	// 注册Prometheus指标端点
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))

	// 注册健康检查端点
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("OK"))
	})

	// 创建HTTP服务器
	m.httpServer = &http.Server{
		Addr:    m.config.Endpoint,
		Handler: mux,
	}

	// 启动HTTP服务器
	go func() {
		if err := m.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// 记录错误
			fmt.Printf("监控服务器错误: %v\n", err)
		}
	}()

	m.running = true
	return nil
}

// Stop 停止监控服务
func (m *PrometheusMonitor) Stop() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.running {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.config.CollectInterval)
	defer cancel()

	if err := m.httpServer.Shutdown(ctx); err != nil {
		return err
	}

	m.running = false
	return nil
}
