package reporter

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
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

	testMode         string  // solo | distributed
	agentMode        *string // strict | any
	distributionMode string
}

func NewCLIReporter(
	metrics *controller.AggregatedMetrics,
	agents *controller.AgentMap,
	testRequest controller.TestRunRequest,
	testInfo *controller.TestInfo,
	updateDuration time.Duration,
	agentMode *string,
	distributionMode string,
) *CLIReporter {
	reporter := &CLIReporter{
		metrics:        metrics,
		agents:         agents,
		testRequest:    testRequest,
		testInfo:       testInfo,
		updateDuration: updateDuration,
		agentMode:      agentMode,
		distributionMode: distributionMode,
	}
	reporter.testMode = SOLO_MODE
	if agents.AgentsCount > 0 {
		reporter.testMode = DISTRIBUTED_MODE
	}

	return reporter
}

func (r *CLIReporter) StartLiveReporting(ctx context.Context, duration int) {
	r.intro()

	switch r.testMode {
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
			elapsed := countTestDurationWithContext(ctx, startTime, time.Duration(duration))
			area.Update(r.renderUI(snap, elapsed, duration, true))
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

	// Фаза ожидания начала теста
	// обновляем только таблицу с агентами
	testStarted := false
	for !testStarted {
		select {
		case <-ctx.Done():
			// Ждём окончания работы всех воркеров агентов
			r.testInfo.WaitForWorkers()
			area.Update(r.renderUI(snap, 0, duration, false))
			return
		case <-r.testInfo.Start:
			testStarted = true
		case <-ticker.C:
			area.Update(r.renderUI(snap, 0, duration, false))
		}
	}

	// Обновляем всё остальное
	startTime := time.Now()
	for {
		select {
		case <-ctx.Done():
			// Ждём окончания работы всех воркеров агентов
			r.testInfo.WaitForWorkers()

			snap := r.metrics.GetSnapshot()
			elapsed := countTestDurationWithContext(ctx, startTime, time.Duration(duration))
			area.Update(r.renderUI(snap, elapsed, duration, true))
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
	var sb strings.Builder

	// Progress bar
	percent := (elapsed.Seconds() / float64(totalDuration)) * 100
	if percent > 100 {
		percent = 100
	}

	bar := getProgressBarString(int(percent), 50)
	elapsedSec := int(elapsed.Seconds())
	if elapsedSec > totalDuration {
		elapsedSec = totalDuration
	}

	// Формируем верхнюю часть (Progress bar + время)
	// Используем Fprintf прямо в builder
	fmt.Fprintf(&sb, "%s %3.1f%% %ds/%ds\n\n", bar, percent, elapsedSec, totalDuration)

	if r.testMode == DISTRIBUTED_MODE {
		// Таблица статистики с агентами
		statsTable := r.distributedTestTable(r.agents)
		sb.WriteString(statsTable)

		// Общая статистика
		mainStatTable := r.mainStatTable(&snap)
		sb.WriteString("\n")
		sb.WriteString(mainStatTable)
	} else {
		// Таблица статистики соло запуска
		statsTable := r.soloTestTable(&snap)
		sb.WriteString(statsTable)
	}

	// HTTP codes (добавляем разделитель и коды)
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "HTTP Codes:\n"+
		" 1xx: %-5d 2xx: %-5d 3xx: %-5d"+
		" 4xx: %-5d 5xx: %-5d Other: %-5d\n",
		snap.Req1XX, snap.Req2XX, snap.Req3XX,
		snap.Req4XX, snap.Req5XX, snap.OtherCodes)

	if isFinal {
		p50 := r.metrics.CalculatePercentile(0.5)
		p90 := r.metrics.CalculatePercentile(0.9)
		p99 := r.metrics.CalculatePercentile(0.99)

		latencyBlock := fmt.Sprintf("\nLatency Distribution:\n"+
			" P50: %v\n P90: %v\n P99: %v\n",
			time.Duration(p50)*time.Millisecond,
			time.Duration(p90)*time.Millisecond,
			time.Duration(p99)*time.Millisecond)

		sb.WriteString(pterm.FgCyan.Sprint(latencyBlock))

		if r.testMode == DISTRIBUTED_MODE {
			top5Errors := r.RenderDistributedDetailedErrors()
			sb.WriteString(top5Errors)
		} else {
			top10Errors := r.RenderSoloDetailedErrors()
			sb.WriteString(top10Errors)
		}
	}

	return sb.String()
}

func (r *CLIReporter) intro() {
	soloStart := r.testMode == SOLO_MODE

	mode := "Distributed"
	if soloStart {
		mode = "Solo"
	}

	intro := fmt.Sprintf("Load testing %s with %d workers\nTarget RPS: %d\nMethod: %s",
		r.testRequest.URL, r.testRequest.Workers, r.testRequest.TargetRPS, r.testRequest.Method)
	if !soloStart {
		intro = fmt.Sprintf("Load testing %s\nTarget RPS: %d\nAgent mode: %s\nDistribution mode: %s\nMethod: %s",
			r.testRequest.URL, r.testRequest.TargetRPS, *r.agentMode, r.distributionMode, r.testRequest.Method)
	}
	pterm.DefaultBox.WithTitle(fmt.Sprintf("Starting %s Test: <%s>", mode, r.testRequest.TestID)).Println(intro)
}

func (r *CLIReporter) soloTestTable(snap *controller.MetricsSnapshot) string {
	successRate := 0.0
	successRequests := snap.TotalSent - snap.TotalFailed
	if successRequests > 0 {
		successRate = (float64(snap.Req2XX) / float64(successRequests)) * 100
	}
	successRateStr := fmt.Sprintf("%.1f%%", successRate)
	statsTable, _ := pterm.DefaultTable.WithData(pterm.TableData{
		{"Requests Sent", "Success", "Failed", "Success Rate", "Min", "Avg", "Max", "Throughput"},
		{
			fmt.Sprintf("%d", snap.TotalSent),
			pterm.LightGreen(fmt.Sprintf("%d", snap.Req2XX)),
			pterm.LightRed(fmt.Sprintf("%d", snap.TotalFailed)),
			successRateStr,
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
	agentTable := make([][]string, 0, agents.AgentsCount+1)
	agentTable = append(agentTable, []string{"Agent", "Sent", "Failed", "RPS", "Success Rate", "Avg", "Max", "Throughput", "Status"})
	for _, agent := range agents.GetList() {
		agentAddressStr := agent.GetAddress()
		sentStr := pterm.LightGreen(fmt.Sprintf("%d", agent.GetSent()))
		failedStr := pterm.LightRed(fmt.Sprintf("%d", agent.GetFailed()))
		currentRPSStr := fmt.Sprintf("%d", agent.GetCurrentRPS())

		successRate := 0.0
		successRequests := agent.GetSuccessRequests()
		if successRequests > 0 {
			successRate = (float64(agent.Get2XX()) / float64(successRequests)) * 100
		}

		maxLatency := agent.GetMaxLatencyMs()
		if maxLatency < 0 {
			maxLatency = 0
		}

		successRateStr := fmt.Sprintf("%.1f%%", successRate)
		avgStr := fmt.Sprintf("%.2fms", agent.GetAvgLatency())
		maxStr := fmt.Sprintf("%.2fms", maxLatency)
		throughputStr := fmt.Sprintf("%.2f MB", float64(agent.GetBytesSent())/1024/1024)

		agentStatusColoredStr := r.colorStatus(agent.GetStatus())
		agentTable = append(
			agentTable,
			[]string{
				agentAddressStr,
				sentStr,
				failedStr,
				currentRPSStr,
				successRateStr,
				avgStr,
				maxStr,
				throughputStr,
				agentStatusColoredStr,
			},
		)
	}
	statsTable, _ := pterm.DefaultTable.WithData(agentTable).Srender()

	return statsTable
}

func (r *CLIReporter) mainStatTable(snap *controller.MetricsSnapshot) string {
	table, _ := pterm.DefaultTable.WithData(pterm.TableData{
		{"Requests Sent", "Success", "Failed", "Throughput"},
		{
			fmt.Sprintf("%d", snap.TotalSent),
			pterm.LightGreen(fmt.Sprintf("%d", snap.Req2XX)),
			pterm.LightRed(fmt.Sprintf("%d", snap.TotalFailed)),
			fmt.Sprintf("%.2f MB", float64(snap.TotalBytes)/1024/1024),
		},
	}).Srender()

	return table
}

// RednerDistributedDetailedErrors рендерит ошибки в распределённом режиме с агентами
func (r *CLIReporter) RenderDistributedDetailedErrors() string {
	agentList := r.agents.GetList()

	hasErrors := false
	for _, ai := range agentList {
		if len(ai.GetErrorDetail()) > 0 {
			hasErrors = true
			break
		}
	}

	if !hasErrors {
		return ""
	}

	var sb strings.Builder

	sort.Slice(agentList, func(i, j int) bool {
		return agentList[i].GetAddress() < agentList[j].GetAddress()
	})

	sb.WriteString(pterm.Bold.Sprint("\nDetailed Network Errors by Agent:\n"))

	for _, ai := range agentList {
		type errorEntry struct {
			msg   string
			count int64
		}
		var topErrors []errorEntry
		for msg, count := range ai.GetErrorDetail() {
			topErrors = append(topErrors, errorEntry{msg, count})
		}

		// Если ошибок нет, пропускаем агента или пишем "No errors"
		if len(topErrors) == 0 {
			continue
		}

		// Сортируем: сначала те, которых больше
		sort.Slice(topErrors, func(i, j int) bool {
			return topErrors[i].count > topErrors[j].count
		})

		// Заголовок агента
		agentHeader := pterm.FgCyan.Sprintf("➤ %s", ai.GetAddress())
		sb.WriteString(agentHeader + "\n")

		// Берем Top 5
		limit := 5
		if len(topErrors) < 5 {
			limit = len(topErrors)
		}

		for i := 0; i < limit; i++ {
			err := topErrors[i]
			line := fmt.Sprintf("  %s %-5d %s\n",
				pterm.Gray("└─"),
				err.count,
				pterm.FgLightRed.Sprint(err.msg),
			)
			sb.WriteString(line)
		}
	}

	return sb.String()
}

// RenderSoloDetailedErrors рендерит ошибки для режима без агентов
func (r *CLIReporter) RenderSoloDetailedErrors() string {
	errorDetails := r.metrics.GetErrorDetails()
	if len(errorDetails) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(pterm.Bold.Sprint("\nDetailed Network Errors:\n"))

	type errorEntry struct {
		msg   string
		count int64
	}
	var topErrors []errorEntry
	for msg, count := range errorDetails {
		topErrors = append(topErrors, errorEntry{msg, count})
	}

	sort.Slice(topErrors, func(i, j int) bool {
		return topErrors[i].count > topErrors[j].count
	})

	limit := 10
	if len(topErrors) < 10 {
		limit = len(topErrors)
	}

	for i := 0; i < limit; i++ {
		err := topErrors[i]
		line := fmt.Sprintf("  %s %-5d %s\n",
			pterm.Gray("└─"),
			err.count,
			pterm.FgLightRed.Sprint(err.msg),
		)
		sb.WriteString(line)
	}

	return sb.String()
}

func (r *CLIReporter) colorStatus(status string) string {
	switch status {
	case controller.STATUS_OFFLINE, controller.STATUS_FAILED, controller.STATUS_CANCELLED:
		return pterm.LightRed(status)
	case controller.STATUS_SYNCHRONIZING, controller.STATUS_CONNECTING:
		return pterm.LightYellow(status)
	case controller.STATUS_FINISHED, controller.STATUS_WORKING:
		return pterm.LightGreen(status)
	default:
		return status
	}
}

// countTestDurationWithContext считает время, которое нужно отобразить на progress bar
// в зависимости от контекста
// Если контекст завершился по дедлайну, то возвращается полное время
// иначе если контекст завершился по отмене, то возвращается время, которое прошло
func countTestDurationWithContext(
	ctx context.Context,
	startTime time.Time,
	testDuration time.Duration,
) time.Duration {
	elapsed := time.Since(startTime)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		elapsed = testDuration * time.Second
	}

	return elapsed
}
