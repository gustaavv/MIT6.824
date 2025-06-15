package kvraft

import "sync"

type uidGenerator struct {
	mu sync.Mutex
	id int
}

func (ug *uidGenerator) nextUid() int {
	ug.mu.Lock()
	defer ug.mu.Unlock()
	ug.id++
	return ug.id
}

// call this function before print value into log
func logV(value string) string {
	if ENABLE_LOG_VALUE || len(value) <= 30 {
		return value
	} else {
		return "<V>"
	}
}
