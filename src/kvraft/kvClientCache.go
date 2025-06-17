package kvraft

import (
	"log"
	"time"
)

func (ck *Clerk) setRespCache(xid int, value string) {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	ck.respCache[xid] = value
}

func (ck *Clerk) getRespCache(xid int) (value string, existed bool) {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	value, existed = ck.respCache[xid]
	return
}

func (ck *Clerk) setLastXid(xid int) {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	ck.lastXid = max(xid, ck.lastXid)
}

func (ck *Clerk) getLastXid() int {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	return ck.lastXid
}

// This function must be called in a critical section
func (ck *Clerk) trimCache() {
	start := time.Now()
	count := 0
	for xid := range ck.respCache {
		if time.Since(start) >= CACHE_TRIM_MAX_DURATION {
			break
		}

		if xid <= ck.lastXid-CACHE_SIZE {
			delete(ck.respCache, xid)
			count++
		}
	}
	end := time.Now()
	log.Printf("ck %d: trim cache took %.3f seconds (max duration %.3f seconds), delete %v cache items",
		ck.cid, end.Sub(start).Seconds(), CACHE_TRIM_MAX_DURATION.Seconds(), count)
}

func getNextTrimCacheAt() time.Time {
	return time.Now().Add(CACHE_TRIM_FREQUENCY)
}

func (ck *Clerk) TrimCacheTicker() {
	if CACHE_SIZE < 0 {
		return
	}

	for !ck.killed() {
		time.Sleep(TICKER_FREQUENCY_LARGE)

		ck.mu.Lock()
		if ck.lastTrimCacheAt.After(time.Now()) {
			ck.mu.Unlock()
			continue
		}
		ck.trimCache()
		ck.lastTrimCacheAt = getNextTrimCacheAt()
		ck.mu.Unlock()

	}
}
