package main

import (
	"bytes"
	stdsync "sync"
)

type syncBuffer struct {
	mu  stdsync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func pushLoopPendingState(loop *pushLoop) (bool, int) {
	loop.pendingMu.Lock()
	defer loop.pendingMu.Unlock()
	var waiters int
	for i := range loop.pendingTasks {
		waiters += len(loop.pendingTasks[i].waiters)
	}
	return len(loop.pendingTasks) > 0, waiters
}
