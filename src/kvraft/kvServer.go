package kvraft

import (
	"bytes"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"6.824/labgob"
	"6.824/labrpc"
	"6.824/raft"
)

type KVServer struct {
	mu      sync.Mutex
	me      int
	rf      *raft.Raft
	applyCh chan raft.ApplyMsg
	dead    int32 // set by Kill()

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.

	persister          *raft.Persister
	store              map[string]string
	session            session
	startAt            time.Time
	lastConsumedIndex  int
	lastReadSnapshotAt time.Time
}

func (kv *KVServer) getDataSummary() string {
	if !raft.ENABLE_LOG {
		return "<DATA_SUMMARY>"
	}
	kv.mu.Lock()
	defer kv.mu.Unlock()
	return fmt.Sprintf("data summary: lastConsumedIndex: %d, %s", kv.lastConsumedIndex, kv.session.String())
}

// Kill
// the tester calls Kill() when a KVServer instance won't
// be needed again. for your convenience, we supply
// code to set rf.dead (without needing a lock),
// and a killed() method to test rf.dead in
// long-running loops. you can also add your own
// code to Kill(). you're not required to do anything
// about this, but it may be convenient (for example)
// to suppress debug output from a Kill()ed instance.
//
func (kv *KVServer) Kill() {
	atomic.StoreInt32(&kv.dead, 1)
	kv.rf.Kill()
	// Your code here, if desired.

	log.Printf("srv %d: shutting down...", kv.me)
	kv.session.broadcastAllClientSessions()
	log.Printf("srv %d: shutting down all handler goroutines", kv.me)
}

func (kv *KVServer) killed() bool {
	z := atomic.LoadInt32(&kv.dead)
	return z == 1
}

// StartKVServer
// servers[] contains the ports of the set of servers that will cooperate via
// Raft to form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
//
func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *KVServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(KVArgs{})

	logHeader := fmt.Sprintf("srv %d: starting: ", me)

	kv := new(KVServer)
	kv.me = me
	kv.maxraftstate = maxraftstate
	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)
	if maxraftstate < 0 {
		kv.rf.SetEnableSnapshot(false)
	}
	kv.persister = persister
	kv.store = make(map[string]string)
	kv.session.clientSessionMap = make(map[int]*clientSession)
	kv.startAt = time.Now()

	kv.installSnapshot(kv.rf.SnapShot.Data, kv.rf.SnapShot.LastIncludedIndex)

	log.Printf("%sstarted, %s", logHeader, kv.getDataSummary())

	go kv.consumeApplyCh()
	go kv.checkLeaderTicker()
	go kv.checkRaftStateSizeTicker()
	go kv.checkStatusTicker()

	return kv
}

func (kv *KVServer) HandleRequest(args *KVArgs, reply *KVReply) {
	// Your code here.

	start := time.Now()

	logHeader := fmt.Sprintf("srv %d: ", kv.me)
	//log.Printf("%s%s", logHeader, args.String())
	defer func() {
		_, isLeader := kv.rf.GetState()
		if !isLeader {
			reply.Success = false
			reply.Msg = MSG_NOT_LEADER
		} else if kv.maxraftstate > 0 {
			kv.mu.Lock()
			if start.Before(kv.lastReadSnapshotAt) {
				reply.Success = false
				reply.Msg = MSG_READ_SNAPSHOT
			}
			kv.mu.Unlock()
		} else if kv.killed() {
			reply.Success = false
			reply.Msg = MSG_SHUTDOWN
		}
		log.Printf("%sleader handles %s %s", logHeader, args.String(), reply.String())
	}()

	if kv.killed() {
		reply.Success = false
		reply.Msg = MSG_SHUTDOWN
		return
	}

	if _, isLeader := kv.rf.GetState(); !isLeader {
		reply.Success = false
		reply.Msg = MSG_NOT_LEADER
		return
	}

	if !(args.Op == OP_GET || args.Op == OP_PUT || args.Op == OP_APPEND) {
		reply.Success = false
		reply.Msg = MSG_OP_UNSUPPORTED
		return
	}

	cs := kv.session.getClientSession(args.Cid)

	// validate xid and use cache
	lastXid, lastResp := cs.getLastXidAndResp()
	if lastXid > args.Xid {
		reply.Success = false
		reply.Msg = MSG_OLD_XID
		return
	} else if lastXid == args.Xid {
		*reply = lastResp
		return
	}

	kv.mu.Lock()
	lastConsumedIndex := kv.lastConsumedIndex
	kv.mu.Unlock()
	if kv.rf.GetLastIndex()-lastConsumedIndex >= UNAVAILABLE_INDEX_DIFF {
		reply.Success = false
		reply.Msg = MSG_UNAVAILABLE
		return
	}

	_, _, isLeader := kv.rf.Start(*args)
	if !isLeader {
		reply.Success = false
		reply.Msg = MSG_NOT_LEADER
		return
	}

	// wait until the request has been handled
	cs.condMu.Lock()
	for lastXid < args.Xid && !kv.killed() {
		cs.cond.Wait()
		lastXid, lastResp = cs.getLastXidAndResp()
	}
	cs.condMu.Unlock()

	if kv.killed() {
		reply.Success = false
		reply.Msg = MSG_SHUTDOWN
		return
	}

	if lastXid == args.Xid {
		*reply = lastResp
	} else {
		// a greater xid has been handled, suggesting that the client does not send one request at a time
		reply.Success = false
		reply.Msg = MSG_MULTIPLE_XID
	}
}

func (kv *KVServer) consumeApplyCh() {
	for applyMsg := range kv.applyCh {
		//start := time.Now()

		if kv.killed() {
			return
		}

		if applyMsg.SnapshotValid {
			if kv.rf.CondInstallSnapshot(applyMsg.SnapshotTerm, applyMsg.SnapshotIndex,
				applyMsg.SnapshotId, applyMsg.Snapshot) {
				kv.installSnapshot(applyMsg.Snapshot, applyMsg.SnapshotIndex)
			}
			continue
		}

		// this mutex is used to isolate this goroutine from checkRaftStateSizeTicker's goroutine
		kv.mu.Lock()

		if applyMsg.CommandIndex != kv.lastConsumedIndex+1 {
			// this can happen only once after installing a snapshot
			kv.mu.Unlock()
			continue
		}
		kv.lastConsumedIndex = applyMsg.CommandIndex

		if !applyMsg.CommandValid {
			kv.mu.Unlock()
			continue
		}

		// no-op
		if applyMsg.Command == nil {
			kv.mu.Unlock()
			continue
		}

		args := applyMsg.Command.(KVArgs)
		cs := kv.session.getClientSession(args.Cid)
		lastXid, _ := cs.getLastXidAndResp()

		logHeader := fmt.Sprintf("srv %d: consume applyMsg: index: %d: ck %d: xid %d: ",
			kv.me, applyMsg.CommandIndex, args.Cid, args.Xid)

		if lastXid < args.Xid {
			reply := KVReply{Success: true}

			switch args.Op {
			case OP_GET:
				v, ok := kv.store[args.Key]
				if !ok {
					v = ""
				}
				reply.Value = v
			case OP_PUT:
				kv.store[args.Key] = args.Value
			case OP_APPEND:
				v, ok := kv.store[args.Key]
				if !ok {
					v = ""
				}
				kv.store[args.Key] = v + args.Value
			}

			cs.setLastXidAndResp(args.Xid, reply)
			log.Printf("%shandle %s succeeds, key %q, reqValue %q, respValue %q",
				logHeader, args.Op, args.Key, args.Value, logV(reply.Value))
		} else if lastXid > args.Xid {
			log.Printf("%sWARN: lastXid %d > args.Xid %d", logHeader, lastXid, args.Xid)
		}

		cs.condMu.Lock()
		cs.cond.Broadcast()
		cs.condMu.Unlock()

		//log.Printf("%stakes %.4f seconds", logHeader, time.Since(start).Seconds())

		kv.mu.Unlock()
	}
}

func (kv *KVServer) checkLeaderTicker() {
	isLeader := false
	for !kv.killed() {
		time.Sleep(TICKER_FREQUENCY)
		_, isLeader2 := kv.rf.GetState()

		// rf elected as the new leader
		if !isLeader && isLeader2 {
			// start a no-op for quick committing and for avoiding deadlock
			kv.rf.Start(nil)
		}

		isLeader = isLeader2
	}
}

func (kv *KVServer) checkRaftStateSizeTicker() {
	if kv.maxraftstate < 0 {
		return
	}

	for !kv.killed() {
		stateSize := kv.persister.RaftStateSize()
		if stateSize < kv.maxraftstate*95/100 {
			kv.rf.PersisterCondWait()
			continue
		}

		kv.mu.Lock()

		index := kv.lastConsumedIndex
		snapshot := kv.takeSnapshot()
		b := kv.rf.Snapshot(index, snapshot)

		kv.mu.Unlock()
		if b {
			log.Printf("srv %d: take snapshot, index %d, stateSize: %d, maxraftstate: %d, md5: %s",
				kv.me, index, stateSize, kv.maxraftstate, hashToMd5(snapshot))
		}

		time.Sleep(time.Millisecond * 10)
	}
}

type ClientSessionTemp struct {
	Cid      int
	LastXid  int
	LastResp KVReply
}

// This function must be called in a critical section
func (kv *KVServer) takeSnapshot() []byte {
	logHeader := fmt.Sprintf("srv %d: takeSnapshot: ", kv.me)
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)

	if err := e.Encode(kv.store); err != nil {
		log.Fatalf("%sencode kv.store err: %v", logHeader, err)
	}

	clientSessionList := make([]ClientSessionTemp, 0)
	kv.session.mu.Lock()
	for _, cs := range kv.session.clientSessionMap {
		clientSessionList = append(clientSessionList, ClientSessionTemp{
			Cid:     cs.cid,
			LastXid: cs.lastXid,
			LastResp: KVReply{
				Success: cs.lastResp.Success,
				Msg:     cs.lastResp.Msg,
				Value:   cs.lastResp.Value,
			},
		})
	}
	kv.session.mu.Unlock()

	if err := e.Encode(clientSessionList); err != nil {
		log.Fatalf("%sencode kv.session err: %v", logHeader, err)
	}
	//log.Printf("%skv.store: %s\n\nkv.session: %s", logHeader, kv.store, kv.session.String())

	ans := w.Bytes()
	return ans
}

func (kv *KVServer) installSnapshot(snapshotData []byte, lastIncludedIndex int) {
	if snapshotData == nil || len(snapshotData) < 1 { // bootstrap without any state?
		return
	}

	if kv.maxraftstate < 0 {
		return
	}

	logHeader := fmt.Sprintf("srv %d: installSnapshot: ", kv.me)
	kv.mu.Lock()

	if lastIncludedIndex < kv.lastConsumedIndex {
		// must not install old snapshot
		kv.mu.Unlock()
		log.Printf("%snot installed: lastIncludedIndex %d < kv.lastConsumedIndex %d",
			logHeader, lastIncludedIndex, kv.lastConsumedIndex)
		return
	}

	defer kv.mu.Unlock()

	r := bytes.NewBuffer(snapshotData)
	d := labgob.NewDecoder(r)

	var store map[string]string
	var clientSessionList []ClientSessionTemp

	if d.Decode(&store) != nil || d.Decode(&clientSessionList) != nil {
		log.Fatalf("%serror happens when reading snapshot", logHeader)
	} else {
		kv.store = store

		kv.session.mu.Lock()
		kv.session.clientSessionMap = make(map[int]*clientSession)
		for _, cst := range clientSessionList {
			cs := makeClientSession(cst.Cid)
			cs.lastXid = cst.LastXid
			// don't know why this is wrong: cs.lastResp = &cst.LastResp
			// maybe related to Go's memory model. just manually copy the fields
			cs.lastResp = &KVReply{
				Success: cst.LastResp.Success,
				Msg:     cst.LastResp.Msg,
				Value:   cst.LastResp.Value,
			}
			kv.session.clientSessionMap[cs.cid] = cs
		}
		kv.session.mu.Unlock()
	}

	kv.lastConsumedIndex = lastIncludedIndex
	kv.lastReadSnapshotAt = time.Now()

	log.Printf("%sinstalled: lastIncludedIndex %d, md5 %s",
		logHeader, lastIncludedIndex, hashToMd5(snapshotData))
}

func (kv *KVServer) checkStatusTicker() {
	for !kv.killed() {
		time.Sleep(time.Second)

		kv.mu.Lock()
		kv.mu.Unlock()
		kv.session.mu.Lock()
		kv.session.mu.Unlock()

		log.Printf("srv %d: check status: no deadlock", kv.me)
	}
}
