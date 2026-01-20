package agent

import (
	"context"
	"log"
	"math"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/gen/agent"
	commonpb "github.com/tendze/diplom2026_distributed_test_orchestrator/gen/common"
	"github.com/tendze/diplom2026_distributed_test_orchestrator/internal/engine"
)

const (
	MetricsStreamDurtaion = time.Second
)

type AgentService struct {
	agent.UnimplementedAgentServiceServer
}

func (s *AgentService) StartTest(
	req *agent.StartTestRequest,
	stream agent.AgentService_StartTestServer,
) error {
	// Создаем контекст, который отменится либо по длительности теста, либо если клиент (контроллер) отвалится
	ctx, cancel := context.WithTimeout(stream.Context(), time.Duration(req.DurationSeconds)*time.Second)
	defer cancel()

	runner := engine.NewHTTPRunner(req.Url)
	limiter := engine.NewRateLimiter(int(req.Rps))
	metrics := NewLocalMetrics()

	// Динамический расчет воркеров
	// Если RPS маленький (например 10), запустим 5 воркеров.
	// Если большой - ограничиваем сверху, чтобы не убить сам Агент.
	workerCount := int(math.Max(1, math.Min(float64(req.Rps/2), 1000)))

	log.Printf("Starting test: ID=%s, RPS=%d, Workers=%d\n", req.TestId, req.Rps, workerCount)

	// Учитываем время старта теста, указанный контроллером
	// для синхронизированного старта
	startAt := time.UnixMilli(req.StartAtUnixMs)
	delay := time.Until(startAt)
	if delay > 0 {
		log.Printf("Waiting %v for synchronized start...", delay)

		// Прерываемое ожидание
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			log.Println("Test cancelled by by context")
			return ctx.Err()
		}
	}

	var wg sync.WaitGroup

	// Сливаем токены, чтобы первую секунду не было 2x rps
	limiter.Drain()

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runLoad(ctx, runner, limiter, metrics)
		}()
	}

	// Воркер стриминга метрик
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.streamMetrics(ctx, metrics, stream)
	}()

	wg.Wait()

	// TODO: replace with slog
	log.Printf("Test %s finished", req.TestId)
	return nil
}

func (s *AgentService) GetStatus(ctx context.Context, req *agent.GetStatusRequest) (*agent.GetStatusResponse, error) {
	cores, err := cpu.Counts(true)
	if err != nil {
		cores = 1
	}

	vMem, err := mem.VirtualMemory()
	totalMem := uint64(0)
	if err == nil {
		totalMem = vMem.Total
	}

	return &agent.GetStatusResponse{
		CpuCores:    int32(cores),
		TotalMemory: totalMem,
	}, nil
}

func (s *AgentService) runLoad(
	ctx context.Context,
	runner *engine.HTTPRunner,
	limiter *engine.RateLimiter,
	metrics *LocalMetrics,
) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			limiter.Wait(ctx)

			status, latency, bytesSent, err := runner.DoRequest()

			metrics.Add(status, latency, bytesSent, err)
		}
	}
}

func (s *AgentService) streamMetrics(ctx context.Context, metrics *LocalMetrics, stream agent.AgentService_StartTestServer) {
	ticker := time.NewTicker(MetricsStreamDurtaion)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sent, failed, bytesSent, statuses, buckets := metrics.Snapshot()

			metrics := &commonpb.Metrics{
				Rps:        float64(sent),
				Sent:       sent,
				Failed:     failed,
				BytesCount: bytesSent,
				Req_1Xx:    statuses[0],
				Req_2Xx:    statuses[1],
				Req_3Xx:    statuses[2],
				Req_4Xx:    statuses[3],
				Req_5Xx:    statuses[4],
				Buckets:    buckets,
			}

			err := stream.Send(metrics)
			if err != nil {
				return
			}
		}
	}
}
