package controller

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	agentpb "github.com/tendze/diplom2026_distributed_test_orchestrator/gen/agent"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	AvailabilityCheckingTimeout = 3 * time.Second
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

// TestRunRequest группирует параметры запуска теста.
type TestRunRequest struct {
	TestID          string
	URL             string
	TargetRPS       int32
	DurationSeconds int32
}

type agentHandle struct {
	addr  string
	cores int32
}

// Start запускает процесс распределенного тестирования на всех настроенных агентах.
func (o *Orchestrator) Start(ctx context.Context, mode string, req TestRunRequest) error {
	var activeAgents []agentHandle
	var totalCores int32

	// 1. Стадия Discovery и Валидация режима
	for _, addr := range o.agentAddrs {
		handle, err := o.pingAgent(ctx, addr)
		if err != nil {
			if mode == "strict" {
				return fmt.Errorf("strict mode violation: agent %s is unreachable: %w", addr, err)
			}
			log.Printf("Agent %s skipped (any mode): %v", addr, err)
			continue
		}
		activeAgents = append(activeAgents, *handle)
		totalCores += handle.cores
	}

	// Проверка на наличие хотя бы одного живого агента
	if len(activeAgents) == 0 {
		return fmt.Errorf("no available agents to run the test")
	}

	var wg sync.WaitGroup

	for _, agent := range activeAgents {
		// Вес агента = его ядра / суммарные ядра
		weight := float64(agent.cores) / float64(totalCores)
		agentRPS := int32(float64(req.TargetRPS) * weight)
		
		if agentRPS == 0 {
			agentRPS = 1 // Гарантируем минимальную нагрузку
		}

		// Воркер нагрузки и отправки метрик
		wg.Add(1)
		go func(a agentHandle, rps int32) {
			defer wg.Done()
			err := o.manageAgentLifecycle(ctx, req.TestID, a.addr, req.URL, rps, req.DurationSeconds)
			if err != nil {
				log.Printf("Agent %s stream error: %v", a.addr, err)
			}
		}(agent, agentRPS)
	}

	// Ожидание завершения всех потоков метрик
	wg.Wait()
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

// pingAgent выполняет предварительную проверку доступности и мощностей агента.
func (o *Orchestrator) pingAgent(ctx context.Context, addr string) (*agentHandle, error) {
	// Контекст для отмены проверки доступности
	pingCtx, cancel := context.WithTimeout(ctx, AvailabilityCheckingTimeout)
	defer cancel()

	// DialContext для соблюдения таймаута
	// WithBlock() заставляет Dial ждать реального установления соединения
	conn, err := grpc.DialContext(pingCtx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	client := agentpb.NewAgentServiceClient(conn)
	
	// Сбор системных характеристик агента
	status, err := client.GetStatus(pingCtx, &agentpb.GetStatusRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get status: %w", err)
	}

	return &agentHandle{
		addr:  addr,
		cores: status.CpuCores,
	}, nil
}
