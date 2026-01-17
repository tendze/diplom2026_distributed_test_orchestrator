package cli

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/gen/agent"
	agentservice "github.com/tendze/diplom2026_distributed_test_orchestrator/internal/agent"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/internal/config"
	"google.golang.org/grpc"
)

var (
	agentConfig string
	agentListen string
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start a component (agent)",
}

var startAgentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run a DTO agent",
	Run:   startAgent,
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.AddCommand(startAgentCmd)
	startAgentCmd.Flags().StringVar(&agentConfig, "config", "agent.yaml", "Path to agent config")
	startAgentCmd.Flags().StringVar(&agentListen, "listen", "", "Override listen address")
}

func startAgent(cmd *cobra.Command, args []string) {
	// Инициализируем логгер
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	slog.SetDefault(logger)

	// Загрузка конфигурации
	var cfg *config.AgentConfig

	if _, err := os.Stat(agentConfig); err == nil {
		cfg, err = config.Load[config.AgentConfig](agentConfig)
		if err != nil {
			logger.Error("failed to load configuration", slog.String("err", err.Error()))
			return
		}
	} else {
		logger.Warn("config file not found, using defaults")
		cfg = config.DefaultAgentConfig()
	}

	if agentListen != "" {
		cfg.Agent.Listen = agentListen
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
