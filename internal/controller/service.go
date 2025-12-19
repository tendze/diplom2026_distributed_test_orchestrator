package controllerservice

import (
	"context"
	"fmt"
	"log"

	agentpb "github.com/tendze/diplom2026_distributed_test_orchestrator/gen/agent"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/gen/controller"
	"google.golang.org/grpc"
)

type ControllerService struct {
	controller.UnimplementedControllerServiceServer
}

func StartTestOnAgent(agentAddr string) {
	conn, err := grpc.NewClient(agentAddr, grpc.WithInsecure())
	if err != nil {
		log.Println("agent dial error:", err)
		return
	}
	defer conn.Close()

	client := agentpb.NewAgentServiceClient(conn)

	stream, err := client.StartTest(
		context.Background(),
		&agentpb.StartTestRequest{
			TestId:          "test-123",
			Url:             "http://example.com",
			Rps:             100,
			DurationSeconds: 10,
		},
	)
	if err != nil {
		log.Println("start test error:", err)
		return
	}

	fmt.Println("Receiving metrics from agent:")

	for {
		m, err := stream.Recv()
		if err != nil {
			fmt.Println("Stream ended:", err)
			break
		}

		fmt.Printf("RPS=%v latency=%vms sent=%v failed=%v\n",
			m.Rps, m.LatencyMs, m.Sent, m.Failed)
	}
}
