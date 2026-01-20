package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/internal/cli/reporter"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/internal/config"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/internal/controller"
)

var runTestMode string

var runTestCmd = &cobra.Command{
	Use:   "run-test [scenario_file]",
	Short: "Start the load test with controller",
	Args:  cobra.ExactArgs(1),
	Run:   runTest,
}

func init() {
	rootCmd.AddCommand(runTestCmd)
}

func runTest(cmd *cobra.Command, args []string) {
	testScenarioPath := args[0]

	// Загрузка конфигурации
	cfg, err := config.Load[config.ControllerConfig](testScenarioPath)
	if err != nil {
		fmt.Printf("failed to load configuration: %s", err.Error())
		return
	}

	if err = cfg.Validate(); err != nil {
		fmt.Printf("invalid config format: %s", err.Error())
		return
	}

	runTestMode = "solo"
	if len(cfg.Agents.Targets) > 0 {
		runTestMode = "distributed"
	}

	// Инициализация агрегатора метрик
	metrics := controller.NewAggregatedMetrics()

	// Инициализация мапы для агентов
	agentMap := controller.NewAgentMap(cfg.Agents.Targets)

	// Инициализация оповещателя
	testInfo := controller.NewTestInfo()

	// Инициализация оркестратора
	orchestrator := controller.NewOrchestrator(agentMap, metrics, testInfo)

	correctWorkers := recountWorkers(cfg.Test.Workers, cfg.Test.TargetRPS)

	runRequest := controller.TestRunRequest{
		TestID:          cfg.Test.ID,
		URL:             cfg.Test.URL,
		TargetRPS:       cfg.Test.TargetRPS,
		DurationSeconds: cfg.Test.DurationSeconds,
		Workers:         correctWorkers,
	}

	// Настройка контекста для корректного завершения (Graceful Shutdown) с сигналом
	baseCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Настройка кондекста для корректного завершения с длительностью тестов
	testDuration := countTestDuration(cfg.Test.DurationSeconds)
	ctx, cancel := context.WithTimeout(baseCtx, testDuration)
	defer cancel()

	var wg sync.WaitGroup

	// Тест
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if err != nil {
				cancel()
			}
		}()

		if agentMap.AgentsCount == 0 {
			err = orchestrator.StartSolo(ctx, &runRequest)
		} else {
			err = orchestrator.StartTest(
				ctx,
				*cfg.Agents.Mode,
				runRequest,
			)
		}
	}()

	// CLI
	wg.Add(1)

	rep := reporter.NewCLIReporter(metrics, agentMap, runRequest, testInfo, 150*time.Millisecond, *cfg.Agents.Mode)
	go func() {
		defer wg.Done()
		rep.StartLiveReporting(ctx, int(cfg.Test.DurationSeconds))
	}()

	wg.Wait()

	if err != nil {
		fmt.Printf("Test execution failed: %s", err.Error())
		return
	}
}

// recountWorkers пересчитывает количество воркеров, когда пользователь не указал своё количество
func recountWorkers(customWorkers, targetRPS int32) int32 {
	workerCount := customWorkers
	if workerCount <= 0 {
		workerCount = targetRPS / 2
		if workerCount < 10 {
			workerCount = 10
		}
		if workerCount > 500 {
			workerCount = 500
		}
	}

	return workerCount
}

// countTestDuration считает количество времени, нужного для выполнения теста
// в зависимости от режима соло или распределённо
func countTestDuration(testDuration int32) time.Duration {
	if runTestMode == "solo" {
		return time.Duration(testDuration) * time.Second
	}

	return time.Duration(testDuration)*time.Second + controller.SyncStartDuration
}
