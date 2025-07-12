package shardkv

//
// client code to talk to a sharded key/value service.
//
// the client first talks to the shardctrler to find out
// the assignment of shards (keys) to groups, and then
// talks to the group that holds the key's shard.
//

import (
	"6.824/atopraft"
	"6.824/labrpc"
	"6.824/shardctrler"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type Clerk struct {
	scClerk  *shardctrler.Clerk
	config   shardctrler.Config
	make_end func(string) *labrpc.ClientEnd
	// You will have to modify this struct.

	Cid               int
	mu                sync.Mutex
	dead              int32 // set by Kill()
	skvClerkMap       map[int]*atopraft.BaseClerk
	skvConfig         *SKVConfig
	xidGenerator      atopraft.UidGenerator
	lastQueryConfigAt time.Time
}

func (ck *Clerk) Kill() {
	atomic.StoreInt32(&ck.dead, 1)
	ck.scClerk.BaseClerk.Kill()
}

func (ck *Clerk) Killed() bool {
	z := atomic.LoadInt32(&ck.dead)
	return z == 1
}

// MakeClerk
// the tester calls MakeClerk.
//
// ctrlers[] is needed to call shardctrler.MakeClerk().
//
// make_end(servername) turns a server name from a
// Config.Groups[gid][i] into a labrpc.ClientEnd on which you can
// send RPCs.
//
func MakeClerk(ctrlers []*labrpc.ClientEnd, make_end func(string) *labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.scClerk = shardctrler.MakeClerk2(ctrlers, nil)
	ck.make_end = make_end
	// You'll have to add code here.

	ck.Cid = ck.scClerk.BaseClerk.Cid
	ck.skvClerkMap = make(map[int]*atopraft.BaseClerk)
	ck.skvConfig = makeSKVConfig()
	ck.lastQueryConfigAt = time.Now()

	go ck.queryConfigTicker()
	return ck
}

// This function must be called in a critical section
func (ck *Clerk) queryConfigAndUpdate() {
	ck.mu.Unlock()
	newCfg := ck.scClerk.BaseQuery(-1, -1) // query only once
	ck.mu.Lock()

	if !(newCfg.Num > 0 && newCfg.Num > ck.config.Num) {
		return
	}
	oldCfg := ck.config

	// delete all old clerks
	for gid := range oldCfg.Groups {
		ck.skvClerkMap[gid].Kill()
	}
	ck.skvClerkMap = make(map[int]*atopraft.BaseClerk)

	// add new clerks
	baseConfig := makeSKVConfig().BC
	for gid, names := range newCfg.Groups {
		servers := make([]*labrpc.ClientEnd, len(names))
		for i, name := range names {
			servers[i] = ck.make_end(name)
		}
		cid := shardctrler.ClerkIdGenerator.NextUid()
		ck.skvClerkMap[gid] = atopraft.MakeBaseClerk(ck, cid, servers, baseConfig, "ShardKV", handleFailureMsg)
	}
	ck.config = newCfg
}

func (ck *Clerk) getNextQueryConfigAt() time.Time {
	return time.Now().Add(ck.skvConfig.CkQueryConfigFrequency)
}

func (ck *Clerk) queryConfigTicker() {
	for !ck.Killed() {
		time.Sleep(ck.skvConfig.BC.TickerFrequency)
		ck.mu.Lock()
		if ck.lastQueryConfigAt.After(time.Now()) {
			ck.mu.Unlock()
			continue
		}
		ck.queryConfigAndUpdate()
		ck.lastQueryConfigAt = ck.getNextQueryConfigAt()
		ck.mu.Unlock()
	}
}

func (ck *Clerk) doRequest(key string, value string, op string) string {
	if !(op == OP_PUT || op == OP_APPEND || op == OP_GET) {
		log.Fatal("op not support", op)
	}
	shard := key2shard(key)
	xid := ck.xidGenerator.NextUid()
	payload := SKVPayLoad{
		ConfigNum: 0,
		Key:       key,
		Value:     value,
	}
	logHeader := fmt.Sprintf("%sCk %d: xid %d: ",
		ck.skvConfig.BC.LogPrefix, ck.Cid, xid)
	log.Printf("%sstart new %s request, key %q", logHeader, op, key)

	for !ck.Killed() {
		ck.mu.Lock()
		gid := ck.config.Shards[shard]
		gck, ok := ck.skvClerkMap[gid]
		if !ok {
			ck.mu.Unlock()
			time.Sleep(ck.skvConfig.BC.TickerFrequency)
			continue
		}
		payload.ConfigNum = ck.config.Num
		ck.mu.Unlock()

		replyValue := gck.DoRequest(&payload, OP_GET, xid, -1)
		if replyValue != nil {
			return string(replyValue.(SKVReplyValue))
		}
	}
	log.Fatalf("UNREACHABLE")
	return ""
}

// Get
// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
// You will have to modify this function.
//
func (ck *Clerk) Get(key string) string {
	return ck.doRequest(key, "", OP_GET)
}

func (ck *Clerk) Put(key string, value string) {
	ck.doRequest(key, value, OP_PUT)
}
func (ck *Clerk) Append(key string, value string) {
	ck.doRequest(key, value, OP_APPEND)
}

func handleFailureMsg(ck *atopraft.BaseClerk, msg string, logHeader string) {
	skvCk := ck.AtopCk.(*Clerk)
	switch msg {
	case MSG_CONFIG_NUM_MISMATCH:
		log.Printf("%sconfig number mismatch", logHeader)
		go func() {
			skvCk.mu.Lock()
			skvCk.queryConfigAndUpdate()
			skvCk.mu.Unlock()
		}()
	case MSG_NOT_MY_SHARD:
		log.Printf("%swrong shard server mapping", logHeader)
		go func() {
			skvCk.mu.Lock()
			skvCk.queryConfigAndUpdate()
			skvCk.mu.Unlock()
		}()
	}
}
