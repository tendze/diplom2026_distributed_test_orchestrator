package controller

import (
	"context"
	"fmt"
	"sync"

	agentpb "github.com/tendze/diplom2026_distributed_test_orchestrator/gen/agent"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Orchestrator выполняет функции управления распределенным тестированием.
type Orchestrator struct {
	agentAddrs []string
	metrics    *AggregatedMetrics
}

// NewOrchestrator создает новый экземпляр оркестратора.
func NewOrchestrator(addrs []string, metrics *AggregatedMetrics) *Orchestrator {
	return &Orchestrator{
		agentAddrs: addrs,
		metrics:    metrics,
	}
}

// Start запускает процесс распределенного тестирования на всех настроенных агентах.
func (o *Orchestrator) Start(ctx context.Context, testID, targetURL string, totalRPS int32, duration int32) error {
	// Расчет RPS на одного агента (базовая реализация равного распределения)
	rpsPerAgent := totalRPS / int32(len(o.agentAddrs))

	var wg sync.WaitGroup
	errChan := make(chan error, len(o.agentAddrs))

	for _, addr := range o.agentAddrs {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			if err := o.manageAgentLifecycle(ctx, testID, address, targetURL, rpsPerAgent, duration); err != nil {
				errChan <- fmt.Errorf("agent %s error: %w", address, err)
			}
		}(addr)
	}

	// Ожидание завершения всех потоков метрик
	wg.Wait()
	close(errChan)

	if len(errChan) > 0 {
		return <-errChan
	}

	return nil
}

// manageAgentLifecycle управляет соединением и приемом данных от конкретного агента.
func (o *Orchestrator) manageAgentLifecycle(ctx context.Context, testID, addr, url string, rps, duration int32) error {
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()

	client := agentpb.NewAgentServiceClient(conn)
	stream, err := client.StartTest(ctx, &agentpb.StartTestRequest{
		TestId:          testID,
		Url:             url,
		Rps:             rps,
		DurationSeconds: duration,
	})
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			msg, err := stream.Recv()
			if err != nil {
				// EOF означает штатное завершение стрима агентом
				return nil
			}

			// Агрегация полученных метрик в общее хранилище
			o.metrics.Merge(
				msg.Sent,
				msg.Failed,
				msg.Req_1Xx,
				msg.Req_2Xx,
				msg.Req_3Xx,
				msg.Req_4Xx,
				msg.Req_5Xx,
				msg.Buckets,
			)
		}
	}
}
