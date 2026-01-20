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
	testInfo       *controller.TestInfo
	updateDuration time.Duration

	mode string // "solo"/"distributed"
}

func NewCLIReporter(
	metrics *controller.AggregatedMetrics,
	agents *controller.AgentMap,
	testRequest controller.TestRunRequest,
	testInfo *controller.TestInfo,
	updateDuration time.Duration,
) *CLIReporter {
	reporter := &CLIReporter{
		metrics:        metrics,
		agents:         agents,
		testRequest:    testRequest,
		testInfo:       testInfo,
		updateDuration: updateDuration,
	}
	reporter.mode = SOLO_MODE
	if agents.AgentsCount > 0 {
		reporter.mode = DISTRIBUTED_MODE
	}

	return reporter
}

func (r *CLIReporter) StartLiveReporting(ctx context.Context, duration int) {
	r.intro()

	switch r.mode {
	case SOLO_MODE:
		r.soloLiveReporting(ctx, duration)
	case DISTRIBUTED_MODE:
		r.distributedLiveReporting(ctx, duration)
	}
}

func (r *CLIReporter) soloLiveReporting(ctx context.Context, duration int) {
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

func (r *CLIReporter) distributedLiveReporting(ctx context.Context, duration int) {
	area, _ := pterm.DefaultArea.Start()
	defer area.Stop()

	ticker := time.NewTicker(r.updateDuration)
	defer ticker.Stop()

	snap := r.metrics.GetSnapshot()
	area.Update(r.renderUI(snap, 0, duration, false))

	testStarted := false
	for !testStarted {
		select {
		case <-ctx.Done():
			return
		case <-r.testInfo.Start:
			testStarted = true
		case <-ticker.C:
			area.Update(r.renderUI(snap, 0, duration, false))
		}
	}

	startTime := time.Now()
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
	// Progress bar
	percent := (elapsed.Seconds() / float64(totalDuration)) * 100
	if percent > 100 {
		percent = 100
	}

	bar := getProgressBarString(int(percent), 50)

	// HTTP codes
	codes := fmt.Sprintf(
		"HTTP Codes:\n"+
			" 1xx: %-5d 2xx: %-5d 3xx: %-5d"+
			" 4xx: %-5d 5xx: %-5d Other: %-5d",
		snap.Req1XX, snap.Req2XX, snap.Req3XX,
		snap.Req4XX, snap.Req5XX, snap.OtherCodes,
	)

	var res string

	elapsedSec := int(elapsed.Seconds())

	if r.mode == DISTRIBUTED_MODE {
		// Таблица статистики с агентами
		statsTable := r.distributedTestTable(r.agents)

		res = fmt.Sprintf(
			"%s %3.1f%% %vs/%vs\n\n%s\n%s",
			bar, percent, elapsedSec, totalDuration, statsTable, codes,
		)

	} else {
		// Таблица статистики соло запуска
		statsTable := r.soloTestTable(&snap)

		res = fmt.Sprintf(
			"%s %3.1f%% %vs/%vs\n\n%s\n%s\n",
			bar, percent, elapsedSec, totalDuration, statsTable, codes,
		)
	}

	if isFinal {
		p50 := r.metrics.CalculatePercentile(0.5)
		p90 := r.metrics.CalculatePercentile(0.9)
		p99 := r.metrics.CalculatePercentile(0.99)

		latencyBlock := fmt.Sprintf("\n\nLatency Distribution:\n"+
			" P50: %v\n P90: %v\n P99: %v\n",
			time.Duration(p50)*time.Millisecond,
			time.Duration(p90)*time.Millisecond,
			time.Duration(p99)*time.Millisecond)

		res += pterm.FgCyan.Sprint(latencyBlock)
	}

	return res
}

func (r *CLIReporter) intro() {
	soloStart := r.mode == SOLO_MODE

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
}

func (r *CLIReporter) soloTestTable(snap *controller.MetricsSnapshot) string {
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

	return statsTable
}

func (r *CLIReporter) distributedTestTable(
	agents *controller.AgentMap,
) string {
	data := make([][]string, 0, agents.AgentsCount+1)
	data = append(data, []string{"Agent", "Sent", "Failed", "Status"})
	for _, agent := range agents.GetList() {
		sentStr := pterm.LightGreen(fmt.Sprintf("%d", agent.GetSent()))
		failedStr := pterm.LightRed(fmt.Sprintf("%d", agent.GetFailed()))
		data = append(data, []string{agent.GetAddress(), sentStr, failedStr, agent.GetStatus()})
	}
	statsTable, _ := pterm.DefaultTable.WithData(data).Srender()

	return statsTable
}
