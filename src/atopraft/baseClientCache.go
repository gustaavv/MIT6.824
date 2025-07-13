package atopraft

import (
	"log"
	"time"
)

func (ck *BaseClerk) setRespCache(xid int, value ReplyValue) {
	ck.Mu.Lock()
	defer ck.Mu.Unlock()
	ck.respCache[xid] = value
}

func (ck *BaseClerk) getRespCache(xid int) (value ReplyValue, existed bool) {
	ck.Mu.Lock()
	defer ck.Mu.Unlock()
	value, existed = ck.respCache[xid]
	return
}

func (ck *BaseClerk) setLastXid(xid int) {
	ck.Mu.Lock()
	defer ck.Mu.Unlock()
	ck.lastXid = Max(xid, ck.lastXid)
}

func (ck *BaseClerk) getLastXid() int {
	ck.Mu.Lock()
	defer ck.Mu.Unlock()
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
	if ck.Config.EnableLog {
		log.Printf("%sCk %d: trim cache took %.3f seconds (max duration %.3f seconds), delete %v cache items",
			ck.Config.LogPrefix, ck.Cid, end.Sub(start).Seconds(), ck.Config.CacheTrimMaxDuration.Seconds(), count)
	}
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

		ck.Mu.Lock()
		if ck.lastTrimCacheAt.After(time.Now()) {
			ck.Mu.Unlock()
			continue
		}
		ck.trimCache()
		ck.lastTrimCacheAt = ck.getNextTrimCacheAt()
		ck.Mu.Unlock()

	}
}
