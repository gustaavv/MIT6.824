package atopraft

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"sync"
)

type UidGenerator struct {
	mu sync.Mutex
	id int
}

func (ug *UidGenerator) NextUid() int {
	ug.mu.Lock()
	defer ug.mu.Unlock()
	ug.id++
	return ug.id
}

// LogV call this function before print value into log
func LogV(value ReplyValue, config *BaseConfig) string {
	s := value.String()
	if config.EnableLogValue || len(s) <= 30 {
		return s
	} else {
		return fmt.Sprintf("<V:L=%d>", len(s))
	}
}

func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func MaxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func HashToMd5(data []byte, config *BaseConfig) string {
	if !config.EnableMD5 {
		return "<MD5>"
	}
	hasher := md5.New()
	hasher.Write(data)

	md5Sum := hasher.Sum(nil)
	md5Hash := hex.EncodeToString(md5Sum)

	return md5Hash
}
