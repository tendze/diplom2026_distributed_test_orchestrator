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
)

var (
	InvalidStatusError = errors.New("invalid status")
)

type AgentInfo struct {
	address string
	status  string
	sent    int64
	failed  int64
	cores   int32

	mu sync.RWMutex
}

// UpdateStatus обновляет статус агента
func (ai *AgentInfo) UpdateStatus(newStatus string) error {
	ai.mu.Lock()
	defer ai.mu.Unlock()

	switch newStatus {
	case STATUS_IDLE, STATUS_CONNECTING, STATUS_SYNCHRONIZING,
		STATUS_WORKING, STATUS_FINISHED, STATUS_FAILED, STATUS_OFFLINE:
		ai.status = newStatus
		return nil
	default:
		return InvalidStatusError
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
 
// GetCores возвращает количество ядер
func (ai *AgentInfo) GetCores() int32 {
	ai.mu.RLock()
	defer ai.mu.RUnlock()
	return ai.cores
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
			address: address,
			status:  STATUS_IDLE,
			sent:    0,
			cores:   0,
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
