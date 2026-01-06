package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/tendze/diplom2026_distributed_test_orchestrator/gen/agent"
	agentservice "github.com/tendze/diplom2026_distributed_test_orchestrator/internal/agent"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/internal/config"
	"google.golang.org/grpc"
)

func main() {
	configPath := flag.String("config", "agent.yaml", "Path to agent configuration file")
	listenOverride := flag.String("listen", "", "override listen address (e.g. :9001)")
	flag.Parse()

	// Загрузка конфигурации
	cfg, err := config.Load[config.AgentConfig](*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if *listenOverride != "" {
		cfg.Agent.Listen = *listenOverride
	}

	// Инициализируем красивый логгер
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	addr := cfg.Agent.Listen
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("failed to listen", "error", err)
		os.Exit(1)
	}

	s := grpc.NewServer()

	agent.RegisterAgentServiceServer(s, &agentservice.AgentService{})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("agent server starting", "addr", addr)
		if err := s.Serve(lis); err != nil {
			slog.Error("failed to serve", "error", err)
		}
	}()

	<-ctx.Done()

	slog.Info("shutting down agent server gracefully...")
	s.GracefulStop()
	slog.Info("agent stopped")
}
