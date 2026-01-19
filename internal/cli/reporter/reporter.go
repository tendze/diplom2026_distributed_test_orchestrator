package reporter

import (
	"context"
	"fmt"
	"time"

	"github.com/pterm/pterm"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/internal/controller"
)

const (
	SOLO_MODE        = "solo"        // Тест в одиночку
	DISTRIBUTED_MODE = "distributed" // Распределённый тест с агентами
)

// CLIReporter выполняет функции отображения метрик, статистики в командной строке
type CLIReporter struct {
	metrics        *controller.AggregatedMetrics
	agents         *controller.AgentMap
	testRequest    controller.TestRunRequest
	updateDuration time.Duration

	mode string // "solo"/"distributed"
}

func NewCLIReporter(
	metrics *controller.AggregatedMetrics,
	agents *controller.AgentMap,
	testRequest controller.TestRunRequest,
	updateDuration time.Duration,
) *CLIReporter {
	reporter := &CLIReporter{
		metrics:        metrics,
		agents:         agents,
		testRequest:    testRequest,
		updateDuration: updateDuration,
	}
	reporter.mode = SOLO_MODE
	if agents.AgentsCount > 0 {
		reporter.mode = DISTRIBUTED_MODE
	}

	return reporter
}

func (r *CLIReporter) StartLiveReporting(ctx context.Context, duration int) {
	soloStart := r.agents.AgentsCount == 0

	// TODO: вынеси в отдельную функцию
	mode := "Distributed"
	if soloStart {
		mode = "Solo"
	}

	intro := fmt.Sprintf("Load testing %s\nTarget RPS: %d",
		r.testRequest.URL, r.testRequest.TargetRPS)

	if soloStart {
		intro = fmt.Sprintf("Load testing %s with %d workers\nTarget RPS: %d",
			r.testRequest.URL, r.testRequest.Workers, r.testRequest.TargetRPS)
	}
	pterm.DefaultBox.WithTitle(fmt.Sprintf("Starting %s Test: <%s>", mode, r.testRequest.TestID)).Println(intro)

	area, _ := pterm.DefaultArea.Start()
	defer area.Stop()

	startTime := time.Now()
	ticker := time.NewTicker(r.updateDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			snap := r.metrics.GetSnapshot()
			area.Update(r.renderUI(snap, time.Duration(duration)*time.Second, duration, true))
			area.Stop()
			return
		case <-ticker.C:
			elapsed := time.Since(startTime)
			snap := r.metrics.GetSnapshot()
			area.Update(r.renderUI(snap, elapsed, duration, false))
		}
	}
}

func (r *CLIReporter) renderUI(snap controller.MetricsSnapshot, elapsed time.Duration, totalDuration int, isFinal bool) string {
	// progress bar
	percent := (elapsed.Seconds() / float64(totalDuration)) * 100
	if percent > 100 {
		percent = 100
	}

	bar := getProgressBarString(int(percent), 50)

	// Таблица статистики latency
	// Для Avg/Min/Max нам нужно добавить эти поля в AggregatedMetrics,
	// но пока выведем общие данные
	statsTable, _ := pterm.DefaultTable.WithData(pterm.TableData{
		{"Requests Sent", "Success", "Failed", "Min", "Avg", "Max", "Throughput"},
		{
			fmt.Sprintf("%d", snap.TotalSent),
			pterm.LightGreen(fmt.Sprintf("%d", snap.Req2XX)),
			pterm.LightRed(fmt.Sprintf("%d", snap.TotalFailed)),
			fmt.Sprintf("%.2fms", snap.MinLatencyMs),
			fmt.Sprintf("%.2fms", snap.AvgLatencyMs),
			fmt.Sprintf("%.2fms", snap.MaxLatencyMs),
			fmt.Sprintf("%.2f MB", float64(snap.TotalBytes)/1024/1024),
		},
	}).Srender()

	// HTTP codes
	codes := fmt.Sprintf(
		"HTTP Codes:\n"+
			" 1xx: %-5d 2xx: %-5d 3xx: %-5d"+
			" 4xx: %-5d 5xx: %-5d Other: %-5d",
		snap.Req1XX, snap.Req2XX, snap.Req3XX,
		snap.Req4XX, snap.Req5XX, snap.OtherCodes,
	)

	elapsedSec := int(elapsed.Seconds())

	res := fmt.Sprintf(
		"%s %3.1f%% %vs/%vs\n\n%s\n%s",
		bar, percent, elapsedSec, totalDuration, statsTable, codes,
	)

	if isFinal {
		p50 := r.metrics.CalculatePercentile(0.5)
		p90 := r.metrics.CalculatePercentile(0.9)
		p99 := r.metrics.CalculatePercentile(0.99)

		// Используем Box или просто текст вместо Section с #
		latencyBlock := fmt.Sprintf("\n\nLatency Distribution:\n"+
			" P50: %v\n P90: %v\n P99: %v\n\n",
			time.Duration(p50)*time.Millisecond,
			time.Duration(p90)*time.Millisecond,
			time.Duration(p99)*time.Millisecond)

		res += pterm.FgCyan.Sprint(latencyBlock)
	}

	return res
}
