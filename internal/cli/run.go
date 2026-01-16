package cli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
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
	// Инициализируем логгер
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	// Загрузка конфигурации
	cfg, err := config.Load[config.ControllerConfig](testScenarioPath)
	if err != nil {
		logger.Error("failed to load configuration", slog.String("err", err.Error()))
		return
	}

	if err = cfg.Validate(); err != nil {
		logger.Error("invalid config format", slog.String("err", err.Error()))
		return
	}

	// Инициализация агрегатора метрик
	metrics := controller.NewAggregatedMetrics()

	agentAddrs := cfg.Agents.Targets

	// Инициализация оркестратора
	orchestrator := controller.NewOrchestrator(agentAddrs, metrics)

	// Настройка контекста для корректного завершения (Graceful Shutdown)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("Starting distributed test", slog.String("test_id", cfg.Test.ID), slog.String("url", cfg.Test.URL), slog.Int("target_rps", int(cfg.Test.TargetRPS)))

	runRequest := controller.TestRunRequest{
		TestID:          cfg.Test.ID,
		URL:             cfg.Test.URL,
		TargetRPS:       cfg.Test.TargetRPS,
		DurationSeconds: cfg.Test.DurationSeconds,
	}

	// Запуск теста
	err = orchestrator.Start(
		ctx,
		cfg.Agents.Mode,
		runRequest,
	)

	if err != nil {
		logger.Error("Test execution failed: " + err.Error())
	}

	logger.Info("Test completed successfully", slog.String("test_id", cfg.Test.ID), slog.String("url", cfg.Test.URL), slog.Int("target_rps", int(cfg.Test.TargetRPS)))
}
