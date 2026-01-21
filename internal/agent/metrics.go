package agent

import (
	"sync"
	"time"
)

// Стандартные границы корзин в миллисекундах (как в Prometheus)
var defaultBuckets = []int32{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

type LocalMetrics struct {
	mu sync.Mutex

	Sent           int64
	Failed         int64
	BytesSent      uint64
	StatusCounters [5]int64 // 1xx, 2xx, 3xx, 4xx, 5xx

	MaxLatency   float64
	MinLatency   float64
	TotalLatency float64 // Сумма всех latency

	// Храним счетчики по корзинам
	Buckets      map[int32]int64
	ErrorsDetail map[string]int64
}

func NewLocalMetrics() *LocalMetrics {
	m := &LocalMetrics{
		Buckets:      make(map[int32]int64),
		ErrorsDetail: make(map[string]int64),
		MaxLatency:   -1,
		MinLatency:   -1,
	}
	for _, b := range defaultBuckets {
		m.Buckets[b] = 0
	}
	return m
}

func (m *LocalMetrics) Add(status int, latency time.Duration, bytesSent uint64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Sent++
	if err != nil {
		m.Failed++
		errMsg := simplifyError(err)
		m.ErrorsDetail[errMsg]++
		return
	}

	m.BytesSent += bytesSent

	ms := int32(latency.Milliseconds())
	found := false
	for _, b := range defaultBuckets {
		if ms <= b {
			m.Buckets[b]++
			found = true
			break
		}
	}
	if !found {
		m.Buckets[5000]++
	}

	latencyMs := float64(latency.Microseconds()) / 1000.0

	if m.MaxLatency == -1 || latencyMs > m.MaxLatency {
		m.MaxLatency = latencyMs
	}
	if m.MinLatency == -1 || latencyMs < m.MinLatency {
		m.MinLatency = latencyMs
	}

	m.TotalLatency += latencyMs

	idx := status/100 - 1
	if idx >= 0 && idx < 5 {
		m.StatusCounters[idx]++
	}
}

func (m *LocalMetrics) Snapshot() LocalMetricsSnapshop {
	m.mu.Lock()
	defer m.mu.Unlock()

	buckets := make(map[int32]int64)
	for k, v := range m.Buckets {
		buckets[k] = v
		m.Buckets[k] = 0
	}

	errorDetail := make(map[string]int64)
	for errorText, count := range m.ErrorsDetail {
		errorDetail[errorText] = count
		delete(m.ErrorsDetail, errorText)
	}

	snapshot := LocalMetricsSnapshop{
		Sent:           m.Sent,
		Failed:         m.Failed,
		BytesSent:      m.BytesSent,
		StatusCounters: m.StatusCounters,
		MaxLatency:     m.MaxLatency,
		MinLatency:     m.MinLatency,
		TotalLatency:   m.TotalLatency,
		Buckets:        buckets,
		ErrorsDetail:   errorDetail,
	}

	// Очищаем
	m.Sent, m.Failed, m.BytesSent = 0, 0, 0
	m.StatusCounters = [5]int64{}

	m.MinLatency, m.MaxLatency = -1, -1
	m.TotalLatency = 0

	return snapshot
}

type LocalMetricsSnapshop struct {
	Sent           int64
	Failed         int64
	BytesSent      uint64
	StatusCounters [5]int64

	MaxLatency   float64
	MinLatency   float64
	TotalLatency float64

	Buckets      map[int32]int64
	ErrorsDetail map[string]int64
}
