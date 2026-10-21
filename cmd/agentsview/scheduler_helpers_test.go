package main

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func waitForSchedulerCondition(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	waitForSchedulerConditionWithin(t, 2*time.Second, cond, msg)
}
func waitForSchedulerConditionWithin(
	t *testing.T, timeout time.Duration, cond func() bool, msg string,
) {
	t.Helper()
	require.Eventually(t, cond, timeout, time.Millisecond, msg)
}

type recordingEmitter struct {
	mu     sync.Mutex
	scopes []string
}

func (e *recordingEmitter) Emit(scope string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scopes = append(e.scopes, scope)
}

func (e *recordingEmitter) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.scopes)
}
