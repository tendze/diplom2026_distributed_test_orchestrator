package controller

import (
	"errors"
	"sync"
)

const (
	STATUS_IDLE          = "IDLE"       // Ждет команды
	STATUS_CONNECTING    = "CONNECTING" // Пытаемся достучаться
	STATUS_SYNCHRONIZING = "SYNCING"    // Ждет времени старта (SyncStartDuration)
	STATUS_WORKING       = "WORKING"    // Выполняет нагрузку
	STATUS_FINISHED      = "FINISHED"   // Успешно завершил
	STATUS_FAILED        = "FAILED"     // Ошибка (упал стрим или gRPC)
	STATUS_OFFLINE       = "OFFLINE"    // Не ответил на пинг
	STATUS_CANCELLED     = "CANCELED"   // Отменён контекст
)

var (
	AgentDisconnectedError = errors.New("agent disconnected")
	AgentConnectionError   = errors.New("failed to connect to agent")
	ContextCancelled       = errors.New("test stopped due to context cancellation")
)

type AgentInfo struct {
	address    string
	status     string
	cores      int32
	currentRPS int64

	req1XX     int64
	req2XX     int64
	req3XX     int64
	req4XX     int64
	req5XX     int64
	otherCodes int64

	sent         int64
	failed       int64
	maxLatencyMs float64
	minLatencyMs float64
	totalLatency float64
	bytesSent    uint64

	errorDetail map[string]int64

	mu sync.RWMutex
}

// UpdateStatus обновляет статус агента
func (ai *AgentInfo) UpdateStatus(newStatus string) {
	ai.mu.Lock()
	defer ai.mu.Unlock()

	switch newStatus {
	case STATUS_IDLE, STATUS_CONNECTING, STATUS_SYNCHRONIZING,
		STATUS_WORKING, STATUS_FINISHED, STATUS_FAILED, STATUS_OFFLINE, STATUS_CANCELLED:
		ai.status = newStatus
	}
}

// GetAddress возвращает адрес агента
func (ai *AgentInfo) GetAddress() string {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	return ai.address
}

// GetStatus возвращает текущий статус агента
func (ai *AgentInfo) GetStatus() string {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	return ai.status
}

// GetSent возвращает количество отправленных запросов
func (ai *AgentInfo) GetSent() int64 {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	return ai.sent
}

// GetFailed возвращает количество проваленных запросов
func (ai *AgentInfo) GetFailed() int64 {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	return ai.failed
}

// GetCurrentRPS возвращает текущее количество отправленных запросов в секунду
func (ai *AgentInfo) GetCurrentRPS() int64 {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	return ai.currentRPS
}

// GetMaxLatencyMs возвращает максимальную задержку в миллисекундах
func (ai *AgentInfo) GetMaxLatencyMs() float64 {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	return ai.maxLatencyMs
}

// GetMinLatencyMs возвращает минимальную задержку в миллисекундах
func (ai *AgentInfo) GetMinLatencyMs() float64 {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	return ai.minLatencyMs
}

// GetAvgLatency возвращает среднее latency всех запросов
func (ai *AgentInfo) GetAvgLatency() float64 {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	successCount := ai.sent - ai.failed
	if successCount <= 0 {
		return 0
	}
	return ai.totalLatency / float64(successCount)
}

// GetBytesSent возвращает количество байт отправленные этим агентом
func (ai *AgentInfo) GetBytesSent() uint64 {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	return ai.bytesSent
}

// Get2XX возвращает количество кодов 2xx
func (ai *AgentInfo) Get2XX() int64 {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	return ai.req2XX
}

// GetSuccessRequests возвращает количество успешных запросов
func (ai *AgentInfo) GetSuccessRequests() int64 {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	return ai.sent - ai.failed
}

// GetErrorDetail возвращает словарь с ошибками и их количеством
func (ai *AgentInfo) GetErrorDetail() map[string]int64 {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	errorDetail := make(map[string]int64)
	for errorMsg, count := range ai.errorDetail {
		errorDetail[errorMsg] = count
	}
	return errorDetail
}

// AddInfo добавляет метрики для отдельного агента
func (ai *AgentInfo) AddInfo(
	sent, failed int64,
	Req1XX int64,
	Req2XX int64,
	Req3XX int64,
	Req4XX int64,
	Req5XX int64,
	currentRPS int64,
	maxLatency, minLatency, totalLatency float64,
	bytesSent uint64,
	errorDetail map[string]int64,
) {
	ai.mu.Lock()
	defer ai.mu.Unlock()
	ai.sent += sent
	ai.failed += failed
	ai.req1XX += Req1XX
	ai.req2XX += Req2XX
	ai.req3XX += Req3XX
	ai.req4XX += Req4XX
	ai.req5XX += Req5XX
	ai.currentRPS = currentRPS
	if ai.maxLatencyMs == -1 || maxLatency > ai.maxLatencyMs {
		ai.maxLatencyMs = maxLatency
	}
	if ai.minLatencyMs == -1 || minLatency < ai.minLatencyMs {
		ai.minLatencyMs = minLatency
	}
	ai.totalLatency += totalLatency
	ai.bytesSent += bytesSent
	for errorMsg, count := range errorDetail {
		ai.errorDetail[errorMsg] += count
	}
}

type AgentMap struct {
	agentMap    map[string]*AgentInfo
	agentList   []*AgentInfo
	AgentsCount int

	mu sync.RWMutex
}

func NewAgentMap(addrs []string) *AgentMap {
	agentMap := AgentMap{
		agentMap:  make(map[string]*AgentInfo),
		agentList: make([]*AgentInfo, 0, len(addrs)),
	}
	for _, address := range addrs {
		agent := &AgentInfo{
			address:      address,
			status:       STATUS_IDLE,
			maxLatencyMs: -1,
			minLatencyMs: -1,
			errorDetail:  make(map[string]int64),
		}
		agentMap.agentMap[address] = agent

		agentMap.agentList = append(agentMap.agentList, agent)
	}

	agentMap.AgentsCount = len(agentMap.agentMap)

	return &agentMap
}

// GetList возвращает агентов в виде списка AgentInfo
func (am *AgentMap) GetList() []*AgentInfo {
	return am.agentList
}

// GetInfo возвращает конкретную информацию об агенте если он есть
func (am *AgentMap) GetInfo(addr string) (*AgentInfo, bool) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	info, exists := am.agentMap[addr]

	return info, exists
}
