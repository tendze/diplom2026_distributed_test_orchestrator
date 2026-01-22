package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	agentpb "github.com/tendze/diplom2026_distributed_test_orchestrator/gen/agent"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/internal/engine"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const (
	AvailabilityCheckingTimeout = 3 * time.Second
	SyncStartDuration           = 2 * time.Second
	TestOverheadDuration        = 2 * time.Second
)

type httpClient interface {
	DoRequest() (status int, latency time.Duration, size uint64, err error)
}

// Orchestrator выполняет функции управления распределенным и одиночным тестированием.
type Orchestrator struct {
	agents  *AgentMap
	metrics *AggregatedMetrics

	testInfo *TestInfo
}

// NewOrchestrator создает новый экземпляр оркестратора.
func NewOrchestrator(agentMap *AgentMap, metrics *AggregatedMetrics, testInfo *TestInfo) *Orchestrator {
	return &Orchestrator{
		agents:   agentMap,
		metrics:  metrics,
		testInfo: testInfo,
	}
}

// TestRunRequest группирует параметры запуска теста.
type TestRunRequest struct {
	TestID          string
	URL             string
	TargetRPS       int32
	DurationSeconds int32
	Workers         int32
}

type agentHandle struct {
	addr  string
	cores int32
}

// Start запускает процесс распределенного тестирования на всех настроенных агентах.
func (o *Orchestrator) StartTest(ctx context.Context, mode string, req TestRunRequest) error {
	var activeAgents []agentHandle
	var totalCores int32

	// 1. Стадия Discovery и Валидация режима
	for _, agent := range o.agents.GetList() {
		handle, err := o.pingAgent(ctx, agent.GetAddress())
		agent.UpdateStatus(STATUS_CONNECTING)
		if err != nil {
			agent.UpdateStatus(STATUS_OFFLINE)
			if mode == "strict" {
				return fmt.Errorf("strict mode violation: agent %s is unreachable: %w", agent.GetAddress(), err)
			}
			continue
		}
		agent.UpdateStatus(STATUS_IDLE)
		activeAgents = append(activeAgents, *handle)
		totalCores += handle.cores
	}

	// Проверка на наличие хотя бы одного живого агента
	if len(activeAgents) == 0 {
		return fmt.Errorf("no available agents to run the test")
	}

	// Механизм гарантированного синхронного старта тестов
	// Устанавливаем время старта теста now() + SyncStartDuration
	startTime := time.Now().Add(SyncStartDuration).UnixMilli()

	go o.testInfo.NotifyAtUnixTime(ctx, startTime)

	var wg sync.WaitGroup

	for _, agent := range activeAgents {
		// Вес агента = его ядра / суммарные ядра
		weight := float64(agent.cores) / float64(totalCores)
		agentRPS := int32(float64(req.TargetRPS) * weight)

		if agentRPS == 0 {
			agentRPS = 1 // Гарантируем минимальную нагрузку
		}

		agentInfo, _ := o.agents.GetInfo(agent.addr)
		agentInfo.UpdateStatus(STATUS_SYNCHRONIZING)

		// Воркер нагрузки и отправки метрик
		wg.Add(1)
		o.testInfo.AddWorker(1)
		go func(a agentHandle, rps int32) {
			defer wg.Done()
			defer o.testInfo.WorkerDone()
			err := o.agentWorker(
				ctx,
				req.TestID,
				a.addr,
				req.URL,
				rps,
				req.DurationSeconds,
				startTime,
			)

			if err != nil {
				if errors.Is(err, AgentDisconnectedError) {
					agentInfo.UpdateStatus(STATUS_FAILED)
				}
				if errors.Is(err, ContextCancelled) {
					agentInfo.UpdateStatus(STATUS_CANCELLED)
				}
			} else {
				agentInfo.UpdateStatus(STATUS_FINISHED)
			}
		}(agent, agentRPS)
	}

	// Ожидание завершения всех потоков метрик
	wg.Wait()
	return nil
}

// StartSoloTest запускает процесс нагрузочного тестирования в одноузловом режиме.
// Метод полностью повторяет жизненный цикл распределенного теста, но выполняет его
// в рамках текущего процесса без использования внешних агентов.
func (o *Orchestrator) StartSolo(ctx context.Context, req *TestRunRequest) error {
	testCtx, cancel := context.WithTimeout(ctx, time.Duration(req.DurationSeconds)*time.Second)
	defer cancel()

	runner := engine.NewFastHTTPRunner(req.URL)
	limiter := engine.NewRateLimiter(int(req.TargetRPS))

	// Чтобы не было лишних запросов
	limiter.Drain()

	var wg sync.WaitGroup
	for i := int32(0); i < req.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o.soloWorker(testCtx, limiter, runner)
		}()
	}

	wg.Wait()
	return nil
}

// agentWorker управляет соединением и приемом данных от конкретного агента.
func (o *Orchestrator) agentWorker(
	ctx context.Context,
	testID, addr, url string,
	rps, duration int32,
	startTimeUnix int64,
) error {
	conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return AgentConnectionError
	}
	defer conn.Close()

	agentInfo, _ := o.agents.GetInfo(addr)

	client := agentpb.NewAgentServiceClient(conn)
	stream, err := client.StartTest(ctx, &agentpb.StartTestRequest{
		TestId:          testID,
		Url:             url,
		Rps:             rps,
		DurationSeconds: duration,
		StartAtUnixMs:   startTimeUnix,
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
				if err == io.EOF {
					return nil
				}

				// Либо отменён контекст
				st, ok := status.FromError(err)
				if ok && st.Code() == codes.Canceled {
					return ContextCancelled
				}

				// Дедлайн просрочился - ок
				if ok && st.Code() == codes.DeadlineExceeded {
					return nil
				}

				// Либо слетело соединение с агентом
				return AgentDisconnectedError
			}

			if agentInfo.GetStatus() != STATUS_WORKING {
				agentInfo.UpdateStatus(STATUS_WORKING)
			}

			// Агрегация полученных метрик в общее хранилище
			o.metrics.Merge(
				msg.Sent,
				msg.Failed,
				msg.BytesCount,
				msg.Req_1Xx,
				msg.Req_2Xx,
				msg.Req_3Xx,
				msg.Req_4Xx,
				msg.Req_5Xx,
				msg.Buckets,
				msg.ErrorsDetail,
			)
			agentInfo.AddInfo(
				msg.Sent,
				msg.Failed,
				msg.Req_1Xx,
				msg.Req_2Xx,
				msg.Req_3Xx,
				msg.Req_4Xx,
				msg.Req_5Xx,
				msg.Sent,
				msg.MaxLatencyMs,
				msg.MinLatencyMs,
				msg.TotalLatencyMs,
				msg.BytesCount,
				msg.ErrorsDetail,
			)
		}
	}
}

// soloWorker отвечает за стимуляцию нагрузки при запуске в соло
func (o *Orchestrator) soloWorker(
	ctx context.Context,
	limiter *engine.RateLimiter,
	httpRunner httpClient,
) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := limiter.Wait(ctx); err != nil {
				return
			}
			status, latency, size, err := httpRunner.DoRequest()
			o.metrics.Add(status, latency, size, err)
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
