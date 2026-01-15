package main

import (
	"context"
	"flag"
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

	// Инициализируем логгер
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	// Загрузка конфигурации
	var cfg *config.AgentConfig

	if _, err := os.Stat(*configPath); err == nil {
		cfg, err = config.Load[config.AgentConfig](*configPath)
		if err != nil {
			logger.Error("failed to load configuration", slog.String("err", err.Error()))
			return
		}
	} else {
		logger.Warn("config file not found, using defaults")
		cfg = config.DefaultAgentConfig()
	}

	if *listenOverride != "" {
		cfg.Agent.Listen = *listenOverride
	}

	if err := cfg.Validate(); err != nil {
		logger.Error("invalid config format", slog.String("err", err.Error()))
		return
	}

	addr := cfg.Agent.Listen
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("failed to listen", slog.String("err", err.Error()))
		os.Exit(1)
	}

	// Сервер

	s := grpc.NewServer()

	agent.RegisterAgentServiceServer(s, &agentservice.AgentService{})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("agent server starting", slog.String("addr", addr))
		if err := s.Serve(lis); err != nil {
			logger.Error("failed to serve", slog.String("err", err.Error()))
		}
	}()

	<-ctx.Done()

	logger.Info("shutting down agent server gracefully...")
	s.GracefulStop()
	logger.Info("agent stopped")
}
