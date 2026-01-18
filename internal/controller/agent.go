package controller

const (
	STATUS_IDLE          = "IDLE"       // Ждет команды
	STATUS_CONNECTING    = "CONNECTING" // Пытаемся достучаться
	STATUS_SYNCHRONIZING = "SYNCING"    // Ждет времени старта (SyncStartDuration)
	STATUS_WORKING       = "WORKING"    // Выполняет нагрузку
	STATUS_FINISHED      = "FINISHED"   // Успешно завершил
	STATUS_FAILED        = "FAILED"     // Ошибка (упал стрим или gRPC)
	STATUS_OFFLINE       = "OFFLINE"    // Не ответил на пинг
)

type AgentInfo struct {
	Address string
	Status  string
	Sent    int64
	Cores   int32
}

type AgentMap struct {
	agentMap    map[string]AgentInfo
	AgentsCount int
}

func NewAgentMap(addrs []string) *AgentMap {
	agentMap := AgentMap{
		agentMap: make(map[string]AgentInfo),
	}
	for _, address := range addrs {
		agentMap.agentMap[address] = AgentInfo{
			Address: address,
			Status:  STATUS_IDLE,
			Sent:    0,
			Cores:   0,
		}
	}

	agentMap.AgentsCount = len(agentMap.agentMap)

	return &agentMap
}

func (am *AgentMap) GetList() []AgentInfo {
	res := make([]AgentInfo, 0, len(am.agentMap))
	for _, agentInfo := range am.agentMap {
		res = append(res, agentInfo)
	}

	return res
}

func (am *AgentMap) GetInfo(addr string) (AgentInfo, bool) {
	info, exists := am.agentMap[addr]
	return info, exists
}
