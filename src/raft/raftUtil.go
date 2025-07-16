package raft

import (
	"errors"
	"os"
	"sync"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !errors.Is(err, os.ErrNotExist)
}

////////////////////////////////////////////////////////

var nextTraceId = 0
var traceIdLock sync.Mutex

func getNextTraceId() int {
	if !ENABLE_TRACE_ID {
		return 0
	}
	traceIdLock.Lock()
	defer traceIdLock.Unlock()
	nextTraceId++
	return nextTraceId
}

////////////////////////////////////////////////////////

var nextSnapshotId = 0
var snapshotIdLock sync.Mutex

func getNextSnapshotId() int {
	if !ENABLE_SNAPSHOT_ID {
		return 0
	}
	snapshotIdLock.Lock()
	defer snapshotIdLock.Unlock()
	nextSnapshotId++
	return nextSnapshotId
}
