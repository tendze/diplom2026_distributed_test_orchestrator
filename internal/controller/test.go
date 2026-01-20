package controller

import (
	"context"
	"time"
)


// TestInfo - вспомогательная структура для оповещения об старте теста
type TestInfo struct {
	Start chan struct{} // Канал для оповещения
}

func NewTestInfo() *TestInfo {
	testInfo := &TestInfo{
		Start: make(chan struct{}, 1),
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
