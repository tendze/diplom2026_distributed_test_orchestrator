package controller

import (
	"context"
	"sync"
	"time"
)

// TestInfo - вспомогательная структура с информацией о тесте
type TestInfo struct {
	Start chan struct{}   // Канал для оповещения
	wg    *sync.WaitGroup // WaitGroup для синхронизации всех агентов - для правильного отображения таблицы с агентами
}

func NewTestInfo() *TestInfo {
	testInfo := &TestInfo{
		Start: make(chan struct{}, 1),
		wg:    &sync.WaitGroup{},
	}
	return testInfo
}

// NotifyAtUnixTime отправляет значение в канал start в указанное unix-время
func (ti *TestInfo) NotifyAtUnixTime(ctx context.Context, unix int64) {
	targetTime := time.UnixMilli(unix)

	timer := time.NewTimer(time.Until(targetTime))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		select {
		case ti.Start <- struct{}{}:
		default:
		}
	}
}

func (ti *TestInfo) AddWorker(agentWorkersCount int) {
	ti.wg.Add(agentWorkersCount)
}

func (ti *TestInfo) WorkerDone() {
	ti.wg.Done()
}

func (ti *TestInfo) WaitForWorkers() {
	ti.wg.Wait()
}
