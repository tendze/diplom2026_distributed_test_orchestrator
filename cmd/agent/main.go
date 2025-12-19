package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/tendze/diplom2026_distributed_test_orchestrator/gen/agent"
	agentservice "github.com/tendze/diplom2026_distributed_test_orchestrator/internal/agent"
	"google.golang.org/grpc"
)

func main() {
	lis, err := net.Listen("tcp", ":9001")
	if err != nil {
		fmt.Println("error creating listener:", err)
		return
	}

	s := grpc.NewServer()

	agent.RegisterAgentServiceServer(s, &agentservice.AgentService{})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	
	log.Println("shutting down agent server")
	s.GracefulStop()
}
