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

	// Инициализация агрегатора метрик
	metrics := controller.NewAggregatedMetrics()

	// Инициализация мапы для агентов
	agentMap := controller.NewAgentMap(cfg.Agents.Targets)

	// Инициализация оркестратора
	orchestrator := controller.NewOrchestrator(agentMap, metrics)

	// Настройка контекста для корректного завершения (Graceful Shutdown) с сигналом
	baseCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Настройка кондекста для корректного завершения с длительностью тестов
	testDuration := time.Duration(cfg.Test.DurationSeconds) * time.Second
	ctx, cancel := context.WithTimeout(baseCtx, testDuration)
	defer cancel()

	correctWorkers := recountWorkers(cfg.Test.Workers, cfg.Test.TargetRPS)

	runRequest := controller.TestRunRequest{
		TestID:          cfg.Test.ID,
		URL:             cfg.Test.URL,
		TargetRPS:       cfg.Test.TargetRPS,
		DurationSeconds: cfg.Test.DurationSeconds,
		Workers:         correctWorkers,
	}

	var wg sync.WaitGroup

	wg.Add(1)
	if agentMap.AgentsCount == 0 {
		// Нагрузочный тест в одиночку
		go func() {
			defer wg.Done()
			err = orchestrator.StartSolo(ctx, &runRequest)
		}()
	} else {
		// Запуск теста с агентами
		go func() {
			defer wg.Done()
			err = orchestrator.StartTest(
				ctx,
				*cfg.Agents.Mode,
				runRequest,
			)
		}()
	}

	// CLI
	wg.Add(1)

	rep := reporter.NewCLIReporter(metrics, agentMap, runRequest, 200*time.Millisecond)
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
