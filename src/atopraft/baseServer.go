package atopraft

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

type BaseServer struct {
	mu      sync.Mutex
	me      int
	Rf      *raft.Raft
	applyCh chan raft.ApplyMsg
	dead    int32 // set by Kill()

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.

	config             *BaseConfig
	persister          *raft.Persister
	Store              interface{}
	session            session
	startAt            time.Time
	lastConsumedIndex  int
	lastReadSnapshotAt time.Time
	allowedOps         map[string]bool
	decodeStore        decodeStore
}

type businessLogic func(srv *BaseServer, args SrvArgs, reply *SrvReply)

type buildStore func() interface{}

type decodeStore func(d *labgob.LabDecoder) (interface{}, error)

func (srv *BaseServer) getDataSummary() string {
	if !raft.ENABLE_LOG {
		return "<DATA_SUMMARY>"
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	return fmt.Sprintf("data summary: lastConsumedIndex: %d, %s",
		srv.lastConsumedIndex, srv.session.String(srv.config))
}

// Kill
// the tester calls Kill() when a BaseServer instance won't
// be needed again. for your convenience, we supply
// code to set Rf.dead (without needing a lock),
// and a Killed() method to test Rf.dead in
// long-running loops. you can also add your own
// code to Kill(). you're not required to do anything
// about this, but it may be convenient (for example)
// to suppress debug output from a Kill()ed instance.
//
func (srv *BaseServer) Kill() {
	atomic.StoreInt32(&srv.dead, 1)
	srv.Rf.Kill()
	// Your code here, if desired.

	logHeader := fmt.Sprintf("%sSrv %d: ", srv.config.LogPrefix, srv.me)
	log.Printf("%sshutting down...", logHeader)
	srv.session.broadcastAllClientSessions()
	log.Printf("%sshutting down all handler goroutines", logHeader)
}

func (srv *BaseServer) killed() bool {
	z := atomic.LoadInt32(&srv.dead)
	return z == 1
}

// StartBaseServer
// servers[] contains the ports of the set of servers that will cooperate via
// Raft to form the fault-tolerant key/value service.
// me is the index of the current server in servers[].
// the k/v server should Store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
//
func StartBaseServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int,
	Ops []string, businessLogic businessLogic, buildStore buildStore, decodeStore decodeStore,
	config *BaseConfig) *BaseServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(SrvArgs{})
	labgob.Register(SrvReply{})

	srv := new(BaseServer)
	srv.me = me
	srv.maxraftstate = maxraftstate
	srv.applyCh = make(chan raft.ApplyMsg)
	srv.Rf = raft.Make(servers, me, persister, srv.applyCh)
	if maxraftstate < 0 {
		srv.Rf.SetEnableSnapshot(false)
	}
	srv.config = config
	srv.persister = persister
	srv.Store = buildStore()
	srv.session.clientSessionMap = make(map[int]*clientSession)
	srv.startAt = time.Now()
	srv.allowedOps = make(map[string]bool)
	for _, op := range Ops {
		srv.allowedOps[op] = true
	}
	srv.decodeStore = decodeStore

	srv.installSnapshot(srv.Rf.SnapShot.Data, srv.Rf.SnapShot.LastIncludedIndex)

	logHeader := fmt.Sprintf("%sSrv %d: starting: ", srv.config.LogPrefix, me)
	log.Printf("%sstarted, %s", logHeader, srv.getDataSummary())

	go srv.consumeApplyCh(businessLogic)
	go srv.checkLeaderTicker()
	go srv.checkRaftStateSizeTicker()
	go srv.checkStatusTicker()

	return srv
}

func (srv *BaseServer) HandleRequest(args *SrvArgs, reply *SrvReply) {
	// Your code here.

	start := time.Now()

	logHeader := fmt.Sprintf("%sSrv %d: ", srv.config.LogPrefix, srv.me)
	//log.Printf("%s%s", logHeader, args.String())
	defer func() {
		_, isLeader := srv.Rf.GetState()
		if !isLeader {
			reply.Success = false
			reply.Msg = MSG_NOT_LEADER
		} else if srv.maxraftstate > 0 {
			srv.mu.Lock()
			if start.Before(srv.lastReadSnapshotAt) {
				reply.Success = false
				reply.Msg = MSG_READ_SNAPSHOT
			}
			srv.mu.Unlock()
		} else if srv.killed() {
			reply.Success = false
			reply.Msg = MSG_SHUTDOWN
		}
		log.Printf("%sleader handles %s %s", logHeader, args.String(), reply.String())
	}()

	if srv.killed() {
		reply.Success = false
		reply.Msg = MSG_SHUTDOWN
		return
	}

	if _, isLeader := srv.Rf.GetState(); !isLeader {
		reply.Success = false
		reply.Msg = MSG_NOT_LEADER
		return
	}

	if _, ok := srv.allowedOps[args.Op]; !ok {
		reply.Success = false
		reply.Msg = MSG_OP_UNSUPPORTED
		return
	}

	cs := srv.session.getClientSession(args.Cid)

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

	srv.mu.Lock()
	lastConsumedIndex := srv.lastConsumedIndex
	srv.mu.Unlock()
	if srv.Rf.GetLastIndex()-lastConsumedIndex >= srv.config.UnavailableIndexDiff {
		reply.Success = false
		reply.Msg = MSG_UNAVAILABLE
		return
	}

	_, _, isLeader := srv.Rf.Start(*args)
	if !isLeader {
		reply.Success = false
		reply.Msg = MSG_NOT_LEADER
		return
	}

	// wait until the request has been handled
	cs.condMu.Lock()
	for lastXid < args.Xid && !srv.killed() {
		cs.cond.Wait()
		lastXid, lastResp = cs.getLastXidAndResp()
	}
	cs.condMu.Unlock()

	if srv.killed() {
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

func (srv *BaseServer) consumeApplyCh(businessLogic businessLogic) {
	for applyMsg := range srv.applyCh {
		//start := time.Now()

		if srv.killed() {
			return
		}

		if applyMsg.SnapshotValid {
			if srv.Rf.CondInstallSnapshot(applyMsg.SnapshotTerm, applyMsg.SnapshotIndex,
				applyMsg.SnapshotId, applyMsg.Snapshot) {
				srv.installSnapshot(applyMsg.Snapshot, applyMsg.SnapshotIndex)
			}
			continue
		}

		// this mutex is used to isolate this goroutine from checkRaftStateSizeTicker's goroutine
		srv.mu.Lock()

		if applyMsg.CommandIndex != srv.lastConsumedIndex+1 {
			// this can happen only once after installing a snapshot
			srv.mu.Unlock()
			continue
		}
		srv.lastConsumedIndex = applyMsg.CommandIndex

		if !applyMsg.CommandValid {
			srv.mu.Unlock()
			continue
		}

		// no-op
		if applyMsg.Command == nil {
			srv.mu.Unlock()
			continue
		}

		args := applyMsg.Command.(SrvArgs)
		cs := srv.session.getClientSession(args.Cid)
		lastXid, _ := cs.getLastXidAndResp()

		logHeader := fmt.Sprintf("%sSrv %d: consume applyMsg: index: %d: ck %d: xid %d: ",
			srv.config.LogPrefix, srv.me, applyMsg.CommandIndex, args.Cid, args.Xid)

		if lastXid < args.Xid {
			reply := SrvReply{Success: true}

			businessLogic(srv, args, &reply)

			cs.setLastXidAndResp(args.Xid, reply)
			log.Printf("%shandle %s succeeds, payload %s, value %s",
				logHeader, args.Op, args.PayLoad, reply.Value)
		} else if lastXid > args.Xid {
			log.Printf("%sWARN: lastXid %d > args.Xid %d", logHeader, lastXid, args.Xid)
		}

		cs.condMu.Lock()
		cs.cond.Broadcast()
		cs.condMu.Unlock()

		//log.Printf("%stakes %.4f seconds", logHeader, time.Since(start).Seconds())

		srv.mu.Unlock()
	}
}

func (srv *BaseServer) checkLeaderTicker() {
	isLeader := false
	for !srv.killed() {
		time.Sleep(srv.config.TickerFrequency)
		_, isLeader2 := srv.Rf.GetState()

		// Rf elected as the new leader
		if !isLeader && isLeader2 {
			// start a no-op for quick committing and for avoiding deadlock
			srv.Rf.Start(nil)
		}

		isLeader = isLeader2
	}
}

func (srv *BaseServer) checkRaftStateSizeTicker() {
	if srv.maxraftstate < 0 {
		return
	}

	for !srv.killed() {
		stateSize := srv.persister.RaftStateSize()
		if stateSize < srv.maxraftstate*95/100 {
			srv.Rf.PersisterCondWait()
			continue
		}

		srv.mu.Lock()

		index := srv.lastConsumedIndex
		snapshot := srv.takeSnapshot()
		b := srv.Rf.Snapshot(index, snapshot)

		srv.mu.Unlock()
		if b {
			log.Printf("%sSrv %d: take snapshot, index %d, stateSize: %d, maxraftstate: %d, md5: %s",
				srv.config.LogPrefix, srv.me, index, stateSize, srv.maxraftstate, HashToMd5(snapshot, srv.config))
		}

		time.Sleep(time.Millisecond * 10)
	}
}

type ClientSessionTemp struct {
	Cid      int
	LastXid  int
	LastResp SrvReply
}

// This function must be called in a critical section
func (srv *BaseServer) takeSnapshot() []byte {
	logHeader := fmt.Sprintf("%sSrv %d: takeSnapshot: ", srv.config.LogPrefix, srv.me)
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)

	if err := e.Encode(srv.Store); err != nil {
		log.Fatalf("%sencode srv.Store err: %v", logHeader, err)
	}

	clientSessionList := make([]ClientSessionTemp, 0)
	srv.session.mu.Lock()
	for _, cs := range srv.session.clientSessionMap {
		var value interface{} = nil
		if cs.lastResp.Value != nil {
			value = cs.lastResp.Value.(ReplyValue).Clone()
		}
		clientSessionList = append(clientSessionList, ClientSessionTemp{
			Cid:     cs.cid,
			LastXid: cs.lastXid,
			LastResp: SrvReply{
				Success: cs.lastResp.Success,
				Msg:     cs.lastResp.Msg,
				Value:   value,
			},
		})
	}
	srv.session.mu.Unlock()

	if err := e.Encode(clientSessionList); err != nil {
		log.Fatalf("%sencode srv.session err: %v", logHeader, err)
	}
	//log.Printf("%skv.Store: %s\n\nsrv.session: %s", logHeader, srv.Store, srv.session.String())

	ans := w.Bytes()
	return ans
}

func (srv *BaseServer) installSnapshot(snapshotData []byte, lastIncludedIndex int) {
	if snapshotData == nil || len(snapshotData) < 1 { // bootstrap without any state?
		return
	}

	if srv.maxraftstate < 0 {
		return
	}

	logHeader := fmt.Sprintf("%sSrv %d: installSnapshot: ", srv.config.LogPrefix, srv.me)
	srv.mu.Lock()

	if lastIncludedIndex < srv.lastConsumedIndex {
		// must not install old snapshot
		srv.mu.Unlock()
		log.Printf("%snot installed: lastIncludedIndex %d < srv.lastConsumedIndex %d",
			logHeader, lastIncludedIndex, srv.lastConsumedIndex)
		return
	}

	defer srv.mu.Unlock()

	r := bytes.NewBuffer(snapshotData)
	d := labgob.NewDecoder(r)

	var clientSessionList []ClientSessionTemp

	store, err := srv.decodeStore(d)

	if err != nil || d.Decode(&clientSessionList) != nil {
		log.Fatalf("%serror happens when reading snapshot", logHeader)
	} else {
		srv.Store = store

		srv.session.mu.Lock()
		srv.session.clientSessionMap = make(map[int]*clientSession)
		for _, cst := range clientSessionList {
			cs := makeClientSession(cst.Cid)
			cs.lastXid = cst.LastXid
			var value interface{} = nil
			if cst.LastResp.Value != nil {
				value = cst.LastResp.Value.(ReplyValue).Clone()
			}
			// don't know why this is wrong: cs.lastResp = &cst.LastResp
			// maybe related to Go's memory model. just manually copy the fields
			cs.lastResp = &SrvReply{
				Success: cst.LastResp.Success,
				Msg:     cst.LastResp.Msg,
				Value:   value,
			}
			srv.session.clientSessionMap[cs.cid] = cs
		}
		srv.session.mu.Unlock()
	}

	srv.lastConsumedIndex = lastIncludedIndex
	srv.lastReadSnapshotAt = time.Now()

	log.Printf("%sinstalled: lastIncludedIndex %d, md5 %s",
		logHeader, lastIncludedIndex, HashToMd5(snapshotData, srv.config))
}

func (srv *BaseServer) checkStatusTicker() {
	for !srv.killed() {
		time.Sleep(time.Second)

		srv.mu.Lock()
		srv.mu.Unlock()
		srv.session.mu.Lock()
		srv.session.mu.Unlock()

		log.Printf("%sSrv %d: check status: no deadlock", srv.config.LogPrefix, srv.me)
	}
}
