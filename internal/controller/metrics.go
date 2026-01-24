package controller

import (
	"sort"
	"sync"
	"time"

	"github.com/tendze/diplom2026_distributed_test_orchestrator/internal/lib"
)

// Стандартные границы корзин в миллисекундах (как в Prometheus)
var defaultBuckets = []int32{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

// AggregatedMetrics обеспечивает потокобезопасный сбор результатов тестирования.
type AggregatedMetrics struct {
	mu sync.RWMutex

	Req1XX     int64
	Req2XX     int64
	Req3XX     int64
	Req4XX     int64
	Req5XX     int64
	OtherCodes int64

	totalSent   int64
	totalFailed int64
	totalBytes  uint64

	maxLatencyMs float64
	minLatencyMs float64
	avgLatencyMs float64

	buckets     map[int32]int64
	errorDetail map[string]int64
}

// NewAggregatedMetrics инициализирует структуру агрегированных метрик.
func NewAggregatedMetrics() *AggregatedMetrics {
	return &AggregatedMetrics{
		buckets:      make(map[int32]int64),
		errorDetail:  make(map[string]int64),
		maxLatencyMs: -1,
		minLatencyMs: -1,
	}
}

// Add собирает метрики в соло запуске теста.
func (m *AggregatedMetrics) Add(status int, latency time.Duration, bytesSent uint64, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalSent++
	if err != nil {
		m.totalFailed++
		errorMsg := lib.SimplifyError(err)
		m.errorDetail[errorMsg]++
		return
	}

	latencyMs := float64(latency.Microseconds()) / 1000.0

	m.totalBytes += bytesSent

	ms := int32(latencyMs)
	for _, b := range defaultBuckets {
		if ms <= b {
			m.buckets[b]++
			break
		}
	}

	// Обновляем максимальный latency
	if m.maxLatencyMs == -1 || latencyMs > m.maxLatencyMs {
		m.maxLatencyMs = latencyMs
	}
	// Обновляем минимальный latency
	if m.minLatencyMs == -1 || latencyMs < m.minLatencyMs {
		m.minLatencyMs = latencyMs
	}

	// Успешные запросы для среднего
	successfulReqs := float64(m.totalSent - m.totalFailed)

	// Расчёт средней задержки
	m.avgLatencyMs = m.avgLatencyMs + (float64(latencyMs)-m.avgLatencyMs)/successfulReqs

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
	agentErrorDetail map[string]int64,
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
	for errorMsg, count := range agentErrorDetail {
		am.errorDetail[errorMsg] += count
	}
}

// GetSnapshot возвращает текущие накопленные показатели.
func (am *AggregatedMetrics) GetSnapshot() MetricsSnapshot {
	am.mu.RLock() // Блокируем только на чтение
	defer am.mu.RUnlock()
	errorDetail := make(map[string]int64)
	for errorMsg, count := range am.errorDetail {
		errorDetail[errorMsg] = count
	}

	return MetricsSnapshot{
		Req1XX:       am.Req1XX,
		Req2XX:       am.Req2XX,
		Req3XX:       am.Req3XX,
		Req4XX:       am.Req4XX,
		Req5XX:       am.Req5XX,
		OtherCodes:   am.OtherCodes,
		TotalSent:    am.totalSent,
		TotalFailed:  am.totalFailed,
		TotalBytes:   am.totalBytes,
		MaxLatencyMs: am.maxLatencyMs,
		MinLatencyMs: am.minLatencyMs,
		AvgLatencyMs: am.avgLatencyMs,
		ErrorDetail:  errorDetail,
	}
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

// GetErrorDetails возвращает словарь с ошибками и их количеством
func (am *AggregatedMetrics) GetErrorDetails() map[string]int64 {
	am.mu.RLock()
	defer am.mu.RUnlock()
	errorDetail := make(map[string]int64)
	for errorMsg, count := range am.errorDetail {
		errorDetail[errorMsg] = count
	}
	return errorDetail
}

// MetricsSnapshot — это слепок данных для отображения в UI
type MetricsSnapshot struct {
	Req1XX, Req2XX, Req3XX, Req4XX, Req5XX, OtherCodes int64
	TotalSent, TotalFailed                             int64
	TotalBytes                                         uint64
	MaxLatencyMs                                       float64
	MinLatencyMs                                       float64
	AvgLatencyMs                                       float64
	ErrorDetail                                        map[string]int64
}
