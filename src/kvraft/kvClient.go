package kvraft

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"6.824/labrpc"
)

var clerkIdGenerator uidGenerator

type Clerk struct {
	servers []*labrpc.ClientEnd
	// You will have to modify this struct.

	mu   sync.Mutex
	dead int32 // set by Kill()

	cid int

	// last completed operation
	lastXid int
	// xid -> KVReply.Value
	respCache map[int]string

	tidGenerator uidGenerator
	xidGenerator uidGenerator

	serverStatus            []*ServerStatusReply
	lastQueryServerStatusAt time.Time
	allLeaderIndex          []int // see getPossibleLeaders

	lastTrimCacheAt time.Time
}

func (ck *Clerk) Kill() {
	atomic.StoreInt32(&ck.dead, 1)
	log.Printf("ck %d: shutting down...", ck.cid)
}

func (ck *Clerk) killed() bool {
	z := atomic.LoadInt32(&ck.dead)
	return z == 1
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	ck.servers = servers
	// You'll have to add code here.

	ck.cid = clerkIdGenerator.nextUid()

	ck.respCache = make(map[int]string)

	ck.serverStatus = make([]*ServerStatusReply, len(servers))
	for i := 0; i < len(servers); i++ {
		ck.serverStatus[i] = &ServerStatusReply{}
	}
	ck.lastQueryServerStatusAt = time.Now()

	ck.allLeaderIndex = make([]int, len(servers))
	for i := 0; i < len(servers); i++ {
		ck.allLeaderIndex[i] = i
	}

	ck.lastTrimCacheAt = getNextTrimCacheAt()

	go ck.queryAllServerStatus()

	go ck.queryServerStatusTicker()
	go ck.TrimCacheTicker()

	log.Printf("ck %d: start", ck.cid)

	return ck
}

func (ck *Clerk) doRequest(key string, value string, op string, xid int, count int) string {
	if ck.killed() {
		return ""
	}

	logHeader := fmt.Sprintf("ck %d: xid %d: count %d: ", ck.cid, xid, count)

	if lastXid := ck.getLastXid(); xid <= lastXid {
		resp, existed := ck.getRespCache(xid)
		if existed {
			return resp
		} else {
			log.Fatalf("%sresp not existed in cache (lastXid %d, maybe the cache item is deleted)", logHeader, lastXid)
		}
	}

	replyCh := make(chan string)
	timeout := time.After(REQUEST_TIMEOUT)

	leaders := ck.getPossibleLeaders()
	log.Printf("%spossible leaders: %v", logHeader, leaders)
	for _, i := range leaders {
		i := i
		server := ck.servers[i]
		go func() {
			args := new(KVArgs)
			args.Key = key
			args.Value = value
			args.Op = op
			args.Cid = ck.cid
			args.Xid = xid
			args.Tid = ck.tidGenerator.nextUid()

			reply := new(KVReply)
			ok := server.Call("KVServer.HandleRequest", args, reply)

			logHeader := fmt.Sprintf("ck %d: xid %d: tid %d: count %d: srv %d: ", ck.cid, xid, args.Tid, count, i)

			if !ok {
				return
			}

			if reply.Success {
				ck.setServerStatus(i, args.Tid, true)
				ck.setRespCache(xid, reply.Value)
				ck.setLastXid(xid)
				replyCh <- reply.Value
				log.Printf("%s%s succeeds, key %q, value %q", logHeader, op, key, logV(reply.Value))
			} else {
				switch reply.Msg {
				case MSG_NOT_LEADER:
					ck.setServerStatus(i, args.Tid, false)
				case MSG_OLD_XID:
					log.Printf("%swarn: send request with old xid to leader", logHeader)
					ck.setServerStatus(i, args.Tid, true)
				case MSG_MULTIPLE_XID:
					log.Printf("%swarn: wrong use of client, you should send requests with one xid at a time", logHeader)
					ck.setServerStatus(i, args.Tid, true)
				case MSG_OP_UNSUPPORTED:
					log.Fatalf("%sunsupported operation %s", logHeader, args.Op)
				case MSG_SHUTDOWN:
					log.Printf("%sserver is shutting down", logHeader)
					ck.setServerStatus(i, args.Tid, false)
				case MSG_READ_SNAPSHOT:
					log.Printf("%sserver is reading snapshot", logHeader)
				case MSG_UNAVAILABLE:
					log.Printf("%sserver is unavailable", logHeader)
				}
			}
		}()
	}

	select {
	case v := <-replyCh:
		return v
	case <-timeout:
		log.Printf("%stimeout, resend requests", logHeader)
		//ck.resetPossibleLeaders()
		return ck.doRequest(key, value, op, xid, count+1) // TODO: set max retries?
	}
}

// Get
// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
func (ck *Clerk) Get(key string) string {
	// You will have to modify this function.
	xid := ck.xidGenerator.nextUid()
	logHeader := fmt.Sprintf("ck %d: xid %d: ", ck.cid, xid)
	log.Printf("%sstart new Get request, key %q", logHeader, key)
	return ck.doRequest(key, "", OP_GET, xid, 1)
}

// PutAppend
// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
func (ck *Clerk) PutAppend(key string, value string, op string) {
	if !(op == OP_PUT || op == OP_APPEND) {
		log.Fatal("op not support", op)
	}

	// You will have to modify this function.
	xid := ck.xidGenerator.nextUid()
	logHeader := fmt.Sprintf("ck %d: xid %d: ", ck.cid, xid)
	log.Printf("%sstart new %s request, key %q value %q", logHeader, op, key, logV(value))
	ck.doRequest(key, value, op, xid, 1)
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, OP_PUT)
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, OP_APPEND)
}
