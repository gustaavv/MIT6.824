package atopraft

import (
	"log"
	"time"
)

func (ck *BaseClerk) setRespCache(xid int, value ReplyValue) {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	ck.respCache[xid] = value
}

func (ck *BaseClerk) getRespCache(xid int) (value ReplyValue, existed bool) {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	value, existed = ck.respCache[xid]
	return
}

func (ck *BaseClerk) setLastXid(xid int) {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	ck.lastXid = Max(xid, ck.lastXid)
}

func (ck *BaseClerk) getLastXid() int {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	return ck.lastXid
}

// This function must be called in a critical section
func (ck *BaseClerk) trimCache() {
	start := time.Now()
	count := 0
	for xid := range ck.respCache {
		if time.Since(start) >= ck.Config.CacheTrimMaxDuration {
			break
		}

		if xid <= ck.lastXid-ck.Config.CacheSize {
			delete(ck.respCache, xid)
			count++
		}
	}
	end := time.Now()
	log.Printf("%sCk %d: trim cache took %.3f seconds (max duration %.3f seconds), delete %v cache items",
		ck.Config.LogPrefix, ck.Cid, end.Sub(start).Seconds(), ck.Config.CacheTrimMaxDuration.Seconds(), count)
}

func (ck *BaseClerk) getNextTrimCacheAt() time.Time {
	return time.Now().Add(ck.Config.CacheTrimFrequency)
}

func (ck *BaseClerk) TrimCacheTicker() {
	if ck.Config.CacheSize < 0 {
		return
	}

	for !ck.Killed() {
		time.Sleep(ck.Config.TickerFrequencyLarge)

		ck.mu.Lock()
		if ck.lastTrimCacheAt.After(time.Now()) {
			ck.mu.Unlock()
			continue
		}
		ck.trimCache()
		ck.lastTrimCacheAt = ck.getNextTrimCacheAt()
		ck.mu.Unlock()

	}
}
