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

type httpClient interface {
	DoRequest(context.Context) (status int, latency time.Duration, size uint64, err error)
}

const (
	MetricsStreamDurtaion = time.Second
	SyncStartDuration     = 2 * time.Second
)

type AgentService struct {
	agent.UnimplementedAgentServiceServer
}

func (s *AgentService) StartTest(
	req *agent.StartTestRequest,
	stream agent.AgentService_StartTestServer,
) error {
	runner := engine.NewFastHTTPRunner(req.Url, req.Method, req.Body)
	limiter := engine.NewRateLimiter(int(req.Rps))
	metrics := NewLocalMetrics()

	// Динамический расчет воркеров
	// Если RPS маленький (например 10), запустим 5 воркеров.
	// Если большой - ограничиваем сверху, чтобы не убить сам Агент.
	workerCount := int(math.Max(1, math.Min(float64(req.Rps/2), 1000)))

	log.Printf("Starting test: ID=%s, RPS=%d, Workers=%d\n", req.TestId, req.Rps, workerCount)

	// Создаем контекст, который отменится либо по длительности теста, либо если клиент (контроллер) отвалится
	ctx, cancel := context.WithTimeout(stream.Context(), time.Duration(req.DurationSeconds)*time.Second+SyncStartDuration)
	defer cancel()

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
	runner httpClient,
	limiter *engine.RateLimiter,
	metrics *LocalMetrics,
) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			limiter.Wait(ctx)

			status, latency, bytesSent, err := runner.DoRequest(ctx)

			metrics.Add(status, latency, bytesSent, err)
		}
	}
}

func (s *AgentService) streamMetrics(ctx context.Context, metrics *LocalMetrics, stream agent.AgentService_StartTestServer) {
	ticker := time.NewTicker(MetricsStreamDurtaion)
	defer ticker.Stop()

	sendSnapshot := func() error {
		snapshot := metrics.Snapshot()

		pbMetrics := &commonpb.Metrics{
			Rps:            float64(snapshot.Sent),
			Sent:           snapshot.Sent,
			Failed:         snapshot.Failed,
			BytesCount:     snapshot.BytesSent,
			Req_1Xx:        snapshot.StatusCounters[0],
			Req_2Xx:        snapshot.StatusCounters[1],
			Req_3Xx:        snapshot.StatusCounters[2],
			Req_4Xx:        snapshot.StatusCounters[3],
			Req_5Xx:        snapshot.StatusCounters[4],
			Buckets:        snapshot.Buckets,
			MinLatencyMs:   snapshot.MinLatency,
			MaxLatencyMs:   snapshot.MaxLatency,
			TotalLatencyMs: snapshot.TotalLatency,
			ErrorsDetail:   snapshot.ErrorsDetail,
		}

		return stream.Send(pbMetrics)
	}

	for {
		select {
		case <-ctx.Done():
			_ = sendSnapshot()
			return
		case <-ticker.C:
			if err := sendSnapshot(); err != nil {
				// Если разорвалось соединение, выходим
				return
			}
		}
	}
}
