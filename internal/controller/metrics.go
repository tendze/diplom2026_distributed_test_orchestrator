package controller

import (
	"sort"
	"sync"
	"time"
)

// Стандартные границы корзин в миллисекундах (как в Prometheus)
var defaultBuckets = []int32{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

// AggregatedMetrics обеспечивает потокобезопасный сбор результатов тестирования.
type AggregatedMetrics struct {
	mu          sync.RWMutex
	Req1XX      int64
	Req2XX      int64
	Req3XX      int64
	Req4XX      int64
	Req5XX      int64
	OtherCodes  int64
	totalSent   int64
	totalFailed int64
	totalBytes  uint64
	buckets     map[int32]int64
}

// NewAggregatedMetrics инициализирует структуру агрегированных метрик.
func NewAggregatedMetrics() *AggregatedMetrics {
	return &AggregatedMetrics{
		buckets: make(map[int32]int64),
	}
}

// Add собирает метрики в соло запуске теста.
func (m *AggregatedMetrics) Add(status int, latency time.Duration, bytesSent uint64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalSent++
	if err != nil {
		m.totalFailed++
		return
	}

	m.totalBytes += bytesSent

	ms := int32(latency.Milliseconds())
	for _, b := range defaultBuckets {
		if ms <= b {
			m.buckets[b]++
			break
		}
	}

	code := status / 100
	switch code {
	case 1:
		m.Req1XX++
	case 2:
		m.Req2XX++
	case 3:
		m.Req3XX++
	case 4:
		m.Req4XX++
	case 5:
		m.Req5XX++
	default:
		m.OtherCodes++
	}
}

// Merge выполняет атомарное сложение метрик, полученных от агентов.
func (am *AggregatedMetrics) Merge(
	sent,
	failed int64, bytesSent uint64,
	req1xx, req2xx, req3xx, req4xx, req5xx int64,
	agentBuckets map[int32]int64,
) {
	am.mu.Lock()
	defer am.mu.Unlock()

	am.Req1XX += req1xx
	am.Req2XX += req2xx
	am.Req3XX += req3xx
	am.Req4XX += req4xx
	am.Req5XX += req5xx

	am.totalSent += sent
	am.totalFailed += failed
	am.totalBytes += bytesSent

	for b, count := range agentBuckets {
		am.buckets[b] += count
	}
}

// GetSnapshot возвращает текущие накопленные показатели.
func (am *AggregatedMetrics) GetSnapshot() (int64, int64) {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.totalSent, am.totalFailed
}

// CalculatePercentile вычисляет значение перцентиля на основе агрегированной гистограммы.
// p — значение в диапазоне (0, 1].
func (am *AggregatedMetrics) CalculatePercentile(p float64) int32 {
	am.mu.RLock()
	defer am.mu.RUnlock()

	var totalSamples int64
	for _, count := range am.buckets {
		totalSamples += count
	}

	if totalSamples == 0 {
		return 0
	}

	target := float64(totalSamples) * p
	var cumulativeCount int64

	// Извлечение и сортировка границ корзин для последовательного обхода
	keys := make([]int, 0, len(am.buckets))
	for k := range am.buckets {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)

	for _, k := range keys {
		cumulativeCount += am.buckets[int32(k)]
		if float64(cumulativeCount) >= target {
			return int32(k)
		}
	}

	return 0
}
