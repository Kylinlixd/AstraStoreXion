package monitor

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
)

// TracingType 追踪类型
type TracingType string

const (
	// Jaeger追踪
	JaegerTracing TracingType = "jaeger"
	// Zipkin追踪
	ZipkinTracing TracingType = "zipkin"
)

// TracingConfig 追踪配置
type TracingConfig struct {
	Type        TracingType       // 追踪类型
	ServiceName string            // 服务名称
	Endpoint    string            // 端点
	Environment string            // 环境
	SampleRate  float64           // 采样率
	Tags        map[string]string // 标签
}

// Tracer 追踪器
type Tracer struct {
	config     TracingConfig
	provider   *sdktrace.TracerProvider
	tracer     trace.Tracer
	propagator propagation.TextMapPropagator
}

// NewTracer 创建追踪器
func NewTracer(config TracingConfig) (*Tracer, error) {
	// 创建导出器
	var exporter sdktrace.SpanExporter
	var err error

	switch config.Type {
	case JaegerTracing:
		// 创建Jaeger导出器
		exporter, err = jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(config.Endpoint)))
	case ZipkinTracing:
		// 创建Zipkin导出器
		exporter, err = zipkin.New(config.Endpoint)
	default:
		return nil, fmt.Errorf("不支持的追踪类型: %s", config.Type)
	}

	if err != nil {
		return nil, fmt.Errorf("创建导出器失败: %v", err)
	}

	// 创建资源
	attrs := []attribute.KeyValue{
		semconv.ServiceNameKey.String(config.ServiceName),
		attribute.String("environment", config.Environment),
	}

	// 添加自定义标签
	for k, v := range config.Tags {
		attrs = append(attrs, attribute.String(k, v))
	}

	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		attrs...,
	)

	// 创建提供者
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.SampleRate)),
	)

	// 设置全局提供者
	otel.SetTracerProvider(provider)

	// 设置全局传播器
	propagator := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
	otel.SetTextMapPropagator(propagator)

	// 创建追踪器
	tracer := provider.Tracer(config.ServiceName)

	return &Tracer{
		config:     config,
		provider:   provider,
		tracer:     tracer,
		propagator: propagator,
	}, nil
}

// StartSpan 开始一个Span
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, opts...)
}

// Inject 注入追踪上下文
func (t *Tracer) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	t.propagator.Inject(ctx, carrier)
}

// Extract 提取追踪上下文
func (t *Tracer) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return t.propagator.Extract(ctx, carrier)
}

// Close 关闭追踪器
func (t *Tracer) Close() error {
	return t.provider.Shutdown(context.Background())
}

// SpanFromContext 从上下文获取Span
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// ContextWithSpan 创建带有Span的上下文
func ContextWithSpan(ctx context.Context, span trace.Span) context.Context {
	return trace.ContextWithSpan(ctx, span)
}

// HTTPCarrier HTTP载体
type HTTPCarrier struct {
	Headers map[string]string
}

// Get 获取键值
func (c HTTPCarrier) Get(key string) string {
	return c.Headers[key]
}

// Set 设置键值
func (c HTTPCarrier) Set(key, value string) {
	c.Headers[key] = value
}

// Keys 获取所有键
func (c HTTPCarrier) Keys() []string {
	keys := make([]string, 0, len(c.Headers))
	for k := range c.Headers {
		keys = append(keys, k)
	}
	return keys
}

// NewHTTPCarrier 创建HTTP载体
func NewHTTPCarrier() *HTTPCarrier {
	return &HTTPCarrier{
		Headers: make(map[string]string),
	}
}
