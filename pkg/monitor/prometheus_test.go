package monitor

import "testing"

func TestPrometheusMonitorRecordsMetrics(t *testing.T) {
	monitor := NewPrometheusMonitor(MonitorConfig{})

	tests := []struct {
		name   string
		def    MetricDefinition
		record func() error
	}{
		{
			name: "counter without labels",
			def: MetricDefinition{
				Name: "test_counter_total",
				Type: CounterMetric,
				Help: "test counter",
			},
			record: func() error {
				return monitor.IncCounter("test_counter_total", 2, nil)
			},
		},
		{
			name: "gauge without labels",
			def: MetricDefinition{
				Name: "test_gauge",
				Type: GaugeMetric,
				Help: "test gauge",
			},
			record: func() error {
				return monitor.SetGauge("test_gauge", 3.5, nil)
			},
		},
		{
			name: "histogram without labels",
			def: MetricDefinition{
				Name:    "test_histogram_seconds",
				Type:    HistogramMetric,
				Help:    "test histogram",
				Buckets: []float64{1, 5},
			},
			record: func() error {
				return monitor.Observe("test_histogram_seconds", 4, nil)
			},
		},
		{
			name: "summary without labels",
			def: MetricDefinition{
				Name:       "test_summary_seconds",
				Type:       SummaryMetric,
				Help:       "test summary",
				Objectives: map[float64]float64{0.5: 0.05},
			},
			record: func() error {
				return monitor.Observe("test_summary_seconds", 6, nil)
			},
		},
		{
			name: "counter with labels",
			def: MetricDefinition{
				Name:   "test_labeled_counter_total",
				Type:   CounterMetric,
				Help:   "test labeled counter",
				Labels: []string{"status"},
			},
			record: func() error {
				return monitor.IncCounter("test_labeled_counter_total", 1, map[string]string{"status": "ok"})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := monitor.RegisterMetric(tc.def); err != nil {
				t.Fatalf("RegisterMetric() error = %v", err)
			}

			if err := tc.record(); err != nil {
				t.Fatalf("record metric error = %v", err)
			}
		})
	}
}

func TestPrometheusMonitorAddsGaugeDelta(t *testing.T) {
	monitor := NewPrometheusMonitor(MonitorConfig{})

	if err := monitor.RegisterMetric(MetricDefinition{
		Name: "test_active_requests",
		Type: GaugeMetric,
		Help: "test active requests",
	}); err != nil {
		t.Fatalf("RegisterMetric() error = %v", err)
	}

	if err := monitor.AddGauge("test_active_requests", 1, nil); err != nil {
		t.Fatalf("AddGauge(+1) error = %v", err)
	}
	if err := monitor.AddGauge("test_active_requests", -1, nil); err != nil {
		t.Fatalf("AddGauge(-1) error = %v", err)
	}
}
