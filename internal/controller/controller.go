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
	"github.com/tendze/diplom2026_distributed_test_orchestrator/internal/lib"
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
	DoRequest(context.Context) (status int, latency time.Duration, size uint64, err error)
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
	TestID           string
	URL              string
	Method           string
	Body             string
	TargetRPS        int32
	DurationSeconds  int32
	Workers          int32
	Monitor          bool
	DistributionMode string
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
		var agentRPS int32
		if req.DistributionMode == "equal" {
			// equal - поровну
			agentRPS = req.TargetRPS / int32(len(activeAgents))
		} else {
			// adaptive - пропорционально ядрам
			weight := float64(agent.cores) / float64(totalCores)
			agentRPS = int32(float64(req.TargetRPS) * weight)
		}

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
				req.Monitor,
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

	runner := engine.NewFastHTTPRunner(req.URL, req.Method, req.Body)
	limiter := engine.NewRateLimiter(int(req.TargetRPS))

	// Чтобы не было лишних запросов
	limiter.Drain()

	var wg sync.WaitGroup
	for i := int32(0); i < req.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o.soloWorker(testCtx, req.TestID, limiter, runner, req.Monitor)
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
	monitor bool,
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

			// Отправка метрик в прометеус, если включен режим мониторинга
			if monitor {
				// Статус-коды
				RequestsTotal.WithLabelValues("1xx", testID).Add(float64(msg.Req_1Xx))
				RequestsTotal.WithLabelValues("2xx", testID).Add(float64(msg.Req_2Xx))
				RequestsTotal.WithLabelValues("3xx", testID).Add(float64(msg.Req_3Xx))
				RequestsTotal.WithLabelValues("4xx", testID).Add(float64(msg.Req_4Xx))
				RequestsTotal.WithLabelValues("5xx", testID).Add(float64(msg.Req_5Xx))

				// Трафик
				BytesTotal.WithLabelValues(testID).Add(float64(msg.BytesCount))

				// Конвертация дискретных корзин агента в кумулятивные для Prometheus
				var cumulativeCount float64
				for _, b := range DefaultBuckets {
					cumulativeCount += float64(msg.Buckets[b])
					// Превращаем число 50 в строку "50" для метки le
					leStr := fmt.Sprintf("%d", b)
					LatencyBuckets.WithLabelValues(leStr, testID).Add(cumulativeCount)
				}

				// Prometheus требует корзину "+Inf" (всё, что больше 5000мс)
				// msg.Sent - это общее количество отправленных запросов в этом тике
				LatencyBuckets.WithLabelValues("+Inf", testID).Add(float64(msg.Sent))

				for errorMsg, count := range msg.ErrorsDetail {
					NetworkErrorsTotal.WithLabelValues(errorMsg, testID).Add(float64(count))
				}
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
	testID string,
	limiter *engine.RateLimiter,
	httpRunner httpClient,
	monitor bool,
) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if err := limiter.Wait(ctx); err != nil {
				return
			}
			status, latency, size, err := httpRunner.DoRequest(ctx)
			o.metrics.Add(status, latency, size, err)

			// TODO: async metrics exporter
			if monitor {
				codeStr := fmt.Sprintf("%dxx", status/100)
				if err != nil {
					codeStr = "error"
				}
				RequestsTotal.WithLabelValues(codeStr, testID).Inc()
				BytesTotal.WithLabelValues(testID).Add(float64(size))

				// Инкремент кумулятивных корзин
				ms := int32(latency.Microseconds()) / 1000
				for _, b := range DefaultBuckets {
					// Если latency <= корзины, инкрементируем её
					if ms <= b {
						LatencyBuckets.WithLabelValues(fmt.Sprintf("%d", b), testID).Inc()
					}
				}

				// Всегда инкрементируем +Inf
				LatencyBuckets.WithLabelValues("+Inf", testID).Inc()

				if err != nil {
					NetworkErrorsTotal.WithLabelValues(lib.SimplifyError(err), testID).Inc()
				}
			}
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
