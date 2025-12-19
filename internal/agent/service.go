package agentservice

import (
	"fmt"
	"time"

	"github.com/tendze/diplom2026_distributed_test_orchestrator/gen/agent"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/gen/common"
)

type AgentService struct {
	agent.UnimplementedAgentServiceServer
}

func (s *AgentService) StartTest(
	req *agent.StartTestRequest,
	stream agent.AgentService_StartTestServer,
) error {
	for i := 0; i < 5; i++ {
		metrics := &common.Metrics{
			Rps:       100,
			LatencyMs: 12.5,
			Sent:      int64(i * 10),
			Failed:    int64(i),
		}

		err := stream.Send(metrics)
		if err != nil {
			return err
		}

		time.Sleep(time.Second)
	}

	fmt.Println("Agent finished test:", req.TestId)
	return nil
}
