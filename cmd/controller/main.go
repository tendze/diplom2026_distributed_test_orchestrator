package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/tendze/diplom2026_distributed_test_orchestrator/internal/config"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/internal/controller"
)

func main() {
	configPath := flag.String("config", "controller.yaml", "Path to controller configuration file")
	flag.Parse()

	// Загрузка конфигурации
	cfg, err := config.Load[config.ControllerConfig](*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Инициализация агрегатора метрик
	metrics := controller.NewAggregatedMetrics()

	agentAddrs := cfg.Agents.Targets

	// Инициализация оркестратора
	orchestrator := controller.NewOrchestrator(agentAddrs, metrics)

	// Настройка контекста для корректного завершения (Graceful Shutdown)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("Starting distributed test: URL=%s, TargetRPS=%d", cfg.Test.URL, cfg.Test.TargetRPS)

	// Запуск теста
	err = orchestrator.Start(
		ctx,
		cfg.Test.ID,
		cfg.Test.URL,
		cfg.Test.TargetRPS,
		cfg.Test.DurationSeconds,
	)

	if err != nil {
		log.Fatalf("Test execution failed: %v", err)
	}

	log.Println("Test completed successfully")
}
