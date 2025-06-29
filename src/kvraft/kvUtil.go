package kvraft

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sync"
)

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
		return fmt.Sprintf("<V:L=%d>", len(value))
	}
}

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

func hashToMd5(data []byte) string {
	if !ENABLE_MD5 {
		return "<MD5>"
	}
	hasher := md5.New()
	hasher.Write(data)

	md5Sum := hasher.Sum(nil)
	md5Hash := hex.EncodeToString(md5Sum)

	return md5Hash
}
