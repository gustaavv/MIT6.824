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
	Mu sync.Mutex
	Me int
	// Me should not be used as the id of a server. Instead, use Sid.
	// TODO: make raft use Sid too
	Sid     int
	Rf      *raft.Raft
	applyCh chan raft.ApplyMsg
	dead    int32 // set by Kill()

	maxraftstate int // snapshot if log grows this big

	// Your definitions here.

	AtopSrv            interface{}
	Config             *BaseConfig
	persister          *raft.Persister
	Store              interface{}
	Session            session
	startAt            time.Time
	LastConsumedIndex  int
	lastReadSnapshotAt time.Time
	allowedOps         map[string]bool
	decodeStore        decodeStore
	validateRequest    validateRequest
	unavailableFlag    int32
	consumeCondMu      sync.Mutex
	consumeCond        *sync.Cond
}

type businessLogic func(srv *BaseServer, args SrvArgs, reply *SrvReply, logHeader string)

type buildStore func() interface{}

type decodeStore func(d *labgob.LabDecoder) (interface{}, error)

type validateRequest func(srv *BaseServer, args *SrvArgs, reply *SrvReply) bool

func (srv *BaseServer) getDataSummary() string {
	if !raft.ENABLE_LOG {
		return "<DATA_SUMMARY>"
	}
	srv.Mu.Lock()
	defer srv.Mu.Unlock()
	return fmt.Sprintf("data summary: LastConsumedIndex: %d, %s",
		srv.LastConsumedIndex, srv.Session.String(srv.Config))
}

func (srv *BaseServer) SetUnavailable() {
	atomic.StoreInt32(&srv.unavailableFlag, 1)
}

func (srv *BaseServer) SetAvailable() {
	atomic.StoreInt32(&srv.unavailableFlag, 0)
}

func (srv *BaseServer) CheckAvailable() bool {
	return atomic.LoadInt32(&srv.unavailableFlag) == 0
}

func (srv *BaseServer) ConsumeCondWait() {
	srv.consumeCondMu.Lock()
	srv.consumeCond.Wait()
	srv.consumeCondMu.Unlock()
}

func (srv *BaseServer) ConsumeCondBroadcast() {
	srv.consumeCondMu.Lock()
	srv.consumeCond.Broadcast()
	srv.consumeCondMu.Unlock()
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

	logHeader := fmt.Sprintf("%sSrv %d: ", srv.Config.LogPrefix, srv.Sid)
	if srv.Config.EnableLog {
		log.Printf("%sshutting down...", logHeader)
	}
	srv.Session.broadcastAllClientSessions()
	if srv.Config.EnableLog {
		log.Printf("%sshutting down all handler goroutines", logHeader)
	}
}

func (srv *BaseServer) Killed() bool {
	z := atomic.LoadInt32(&srv.dead)
	return z == 1
}

// StartBaseServer
// Servers[] contains the ports of the set of Servers that will cooperate via
// Raft to form the fault-tolerant key/value service.
// Me is the index of the current server in Servers[].
// the k/v server should Store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
// the k/v server should snapshot when Raft's saved state exceeds maxraftstate bytes,
// in order to allow Raft to garbage-collect its log. if maxraftstate is -1,
// you don't need to snapshot.
// StartKVServer() must return quickly, so it should start goroutines
// for any long-running work.
//
func StartBaseServer(sid int, atopSrv interface{}, servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int,
	Ops []string, businessLogic businessLogic, buildStore buildStore, decodeStore decodeStore,
	validateRequest validateRequest, config *BaseConfig) *BaseServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(SrvArgs{})
	labgob.Register(SrvReply{})

	srv := new(BaseServer)
	srv.AtopSrv = atopSrv
	srv.Sid = sid
	srv.Me = me
	srv.maxraftstate = maxraftstate
	srv.applyCh = make(chan raft.ApplyMsg)
	srv.Rf = raft.Make(servers, me, persister, srv.applyCh)
	if maxraftstate < 0 {
		srv.Rf.SetEnableSnapshot(false)
	}
	srv.Config = config
	srv.persister = persister
	srv.Store = buildStore()
	srv.Session.clientSessionMap = make(map[int]*clientSession)
	srv.startAt = time.Now()
	srv.allowedOps = make(map[string]bool)
	for _, op := range Ops {
		srv.allowedOps[op] = true
	}
	srv.decodeStore = decodeStore
	srv.validateRequest = validateRequest
	srv.consumeCond = sync.NewCond(&srv.consumeCondMu)

	srv.installSnapshot(srv.Rf.SnapShot.Data, srv.Rf.SnapShot.LastIncludedIndex)

	logHeader := fmt.Sprintf("%sSrv %d: starting: ", srv.Config.LogPrefix, sid)
	if srv.Config.EnableLog {
		log.Printf("%sstarted, %s", logHeader, srv.getDataSummary())
	}
	go srv.consumeApplyCh(businessLogic)
	go srv.checkLeaderTicker()
	go srv.checkRaftStateSizeTicker()
	go srv.checkStatusTicker()

	return srv
}

func (srv *BaseServer) HandleRequest(args *SrvArgs, reply *SrvReply) {
	// Your code here.

	start := time.Now()

	logHeader := fmt.Sprintf("%sSrv %d: ", srv.Config.LogPrefix, srv.Sid)
	//log.Printf("%s%s", logHeader, args.String())
	defer func() {
		_, isLeader := srv.Rf.GetState()
		if !isLeader {
			reply.Success = false
			reply.Msg = MSG_NOT_LEADER
		} else if srv.maxraftstate > 0 {
			srv.Mu.Lock()
			if start.Before(srv.lastReadSnapshotAt) {
				reply.Success = false
				reply.Msg = MSG_READ_SNAPSHOT
			}
			srv.Mu.Unlock()
		} else if srv.Killed() {
			reply.Success = false
			reply.Msg = MSG_SHUTDOWN
		}
		if srv.Config.EnableLog {
			log.Printf("%sleader handles %s %s", logHeader, args.String(), reply.String())
		}
	}()

	if srv.Killed() {
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

	// ids < 0 are for internal use. Clients can't use such ids
	if !(args.Cid >= 0 && args.Tid >= 0 && args.Xid >= 0) {
		reply.Success = false
		reply.Msg = MSG_INVALID_ARGS
		return
	}

	if !srv.validateRequest(srv, args, reply) {
		reply.Success = false
		return
	}

	cs := srv.Session.getClientSession(args.Cid)

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

	srv.Mu.Lock()
	lastConsumedIndex := srv.LastConsumedIndex
	srv.Mu.Unlock()
	if srv.Rf.GetLastIndex()-lastConsumedIndex >= srv.Config.UnavailableIndexDiff {
		reply.Success = false
		reply.Msg = MSG_UNAVAILABLE
		return
	}

	if !srv.CheckAvailable() {
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
	for lastXid < args.Xid && !srv.Killed() {
		cs.cond.Wait()
		lastXid, lastResp = cs.getLastXidAndResp()
	}
	cs.condMu.Unlock()

	if srv.Killed() {
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

		if srv.Killed() {
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
		srv.Mu.Lock()

		if applyMsg.CommandIndex != srv.LastConsumedIndex+1 {
			// this can happen only once after installing a snapshot
			srv.Mu.Unlock()
			continue
		}
		srv.LastConsumedIndex = applyMsg.CommandIndex

		if !applyMsg.CommandValid {
			srv.Mu.Unlock()
			srv.ConsumeCondBroadcast()
			continue
		}

		// no-op
		if applyMsg.Command == nil {
			srv.Mu.Unlock()
			srv.ConsumeCondBroadcast()
			continue
		}

		args := applyMsg.Command.(SrvArgs)
		cs := srv.Session.getClientSession(args.Cid)
		lastXid, _ := cs.getLastXidAndResp()

		logHeader := fmt.Sprintf("%sSrv %d: consume applyMsg: index: %d: ck %d: xid %d: ",
			srv.Config.LogPrefix, srv.Sid, applyMsg.CommandIndex, args.Cid, args.Xid)

		// xid < 0 is for internal use
		if lastXid < args.Xid || args.Xid < 0 {
			// assume reply succeeds, but it can fail in businessLogic()
			reply := SrvReply{Success: true}
			businessLogic(srv, args, &reply, logHeader)

			result := "succeeds"
			if reply.Success {
				if args.Xid >= 0 {
					cs.setLastXidAndResp(args.Xid, reply)
				}
			} else {
				result = "fails"
			}
			if srv.Config.EnableLog {
				log.Printf("%shandle %s %s, payload %s, value %s",
					logHeader, args.Op, result, args.PayLoad, reply.Value)
			}
		} else if lastXid > args.Xid {
			if srv.Config.EnableLog {
				log.Printf("%sWARN: lastXid %d > args.Xid %d", logHeader, lastXid, args.Xid)
			}
		}

		cs.condMu.Lock()
		cs.cond.Broadcast()
		cs.condMu.Unlock()

		//log.Printf("%stakes %.4f seconds", logHeader, time.Since(start).Seconds())

		srv.Mu.Unlock()

		srv.ConsumeCondBroadcast()
	}
}

func (srv *BaseServer) checkLeaderTicker() {
	isLeader := false
	for !srv.Killed() {
		time.Sleep(srv.Config.TickerFrequency)
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

	for !srv.Killed() {
		stateSize := srv.persister.RaftStateSize()
		if stateSize < srv.maxraftstate*95/100 {
			srv.Rf.PersisterCondWait()
			continue
		}

		srv.Mu.Lock()

		index := srv.LastConsumedIndex
		snapshot := srv.takeSnapshot()
		b := srv.Rf.Snapshot(index, snapshot)

		srv.Mu.Unlock()
		if b {
			if srv.Config.EnableLog {
				log.Printf("%sSrv %d: take snapshot, index %d, stateSize: %d, maxraftstate: %d, md5: %s",
					srv.Config.LogPrefix, srv.Sid, index, stateSize, srv.maxraftstate, HashToMd5(snapshot, srv.Config))
			}
		}

		time.Sleep(time.Millisecond * 10)
	}
}

// This function must be called in a critical section
func (srv *BaseServer) takeSnapshot() []byte {
	logHeader := fmt.Sprintf("%sSrv %d: takeSnapshot: ", srv.Config.LogPrefix, srv.Sid)
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)

	if err := e.Encode(srv.Store); err != nil {
		log.Fatalf("%sencode srv.Store err: %v", logHeader, err)
	}

	clientSessionList := srv.Session.Clone()

	if err := e.Encode(clientSessionList); err != nil {
		log.Fatalf("%sencode srv.Session err: %v", logHeader, err)
	}
	//log.Printf("%skv.Store: %s\n\nsrv.Session: %s", logHeader, srv.Store, srv.Session.String())

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

	logHeader := fmt.Sprintf("%sSrv %d: installSnapshot: ", srv.Config.LogPrefix, srv.Sid)
	srv.Mu.Lock()

	if lastIncludedIndex < srv.LastConsumedIndex {
		// must not install old snapshot
		srv.Mu.Unlock()
		if srv.Config.EnableLog {
			log.Printf("%snot installed: lastIncludedIndex %d < srv.LastConsumedIndex %d",
				logHeader, lastIncludedIndex, srv.LastConsumedIndex)
		}
		return
	}

	defer srv.Mu.Unlock()

	r := bytes.NewBuffer(snapshotData)
	d := labgob.NewDecoder(r)

	var clientSessionList []ClientSessionTemp

	store, err := srv.decodeStore(d)

	if err != nil || d.Decode(&clientSessionList) != nil {
		log.Fatalf("%serror happens when reading snapshot", logHeader)
	} else {
		srv.Store = store
		srv.Session.Update(clientSessionList)
	}

	srv.LastConsumedIndex = lastIncludedIndex
	srv.lastReadSnapshotAt = time.Now()
	if srv.Config.EnableLog {
		log.Printf("%sinstalled: lastIncludedIndex %d, md5 %s",
			logHeader, lastIncludedIndex, HashToMd5(snapshotData, srv.Config))
	}
}

func (srv *BaseServer) checkStatusTicker() {
	if !srv.Config.EnableCheckStatusTicker {
		return
	}

	for !srv.Killed() {
		time.Sleep(time.Second)

		srv.Mu.Lock()
		srv.Mu.Unlock()
		srv.Session.mu.Lock()
		srv.Session.mu.Unlock()
		if srv.Config.EnableLog {
			log.Printf("%sSrv %d: check status: no deadlock", srv.Config.LogPrefix, srv.Sid)
		}
	}
}

func (srv *BaseServer) WaitUntilEntryConsumed(index int) {
	for {
		srv.Mu.Lock()
		if srv.LastConsumedIndex >= index {
			srv.Mu.Unlock()
			return
		}
		srv.Mu.Unlock()
		srv.ConsumeCondWait()
	}
}

func (srv *BaseServer) CheckLeader() bool {
	_, isLeader := srv.Rf.GetState()
	return isLeader
}
