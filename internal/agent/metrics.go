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
	StatusCounters [5]int64 // 1xx, 2xx, 3xx, 4xx, 5xx

	// Храним счетчики по корзинам
	Buckets map[int32]int64
}

func NewLocalMetrics() *LocalMetrics {
	m := &LocalMetrics{
		Buckets: make(map[int32]int64),
	}
	for _, b := range defaultBuckets {
		m.Buckets[b] = 0
	}
	return m
}

func (m *LocalMetrics) Add(status int, latency time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Sent++
	if err != nil {
		m.Failed++
		return
	}

	ms := int32(latency.Milliseconds())
	for _, b := range defaultBuckets {
		if ms <= b {
			m.Buckets[b]++
			break
		}
	}

	idx := status/100 - 1
	if idx >= 0 && idx < 5 {
		m.StatusCounters[idx]++
	}
}

func (m *LocalMetrics) Snapshot() (
	sent, failed int64,
	statuses [5]int64,
	buckets map[int32]int64,
) {
	m.mu.Lock()
	defer m.mu.Unlock()

	buckets = make(map[int32]int64)
	for k, v := range m.Buckets {
		buckets[k] = v
		m.Buckets[k] = 0
	}

	sent, failed = m.Sent, m.Failed
	statuses = m.StatusCounters

	m.Sent, m.Failed = 0, 0
	m.StatusCounters = [5]int64{}

	return
}


