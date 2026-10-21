package main

import (
	"time"

	"go.kenn.io/agentsview/internal/sync"
)

// notifyTeeEmitter delivers sync events before notifying a background consumer.
type notifyTeeEmitter struct {
	primary sync.Emitter
	notify  func()
}

func (t notifyTeeEmitter) Emit(scope string) {
	t.primary.Emit(scope)
	t.notify()
}

func resetTimer(t *time.Timer, d time.Duration) { t.Reset(d) }
