package kvraft

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"

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
	store   map[string]string
	session session
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
	//close(kv.applyCh)
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

	kv := new(KVServer)
	kv.me = me
	kv.maxraftstate = maxraftstate
	kv.applyCh = make(chan raft.ApplyMsg)
	kv.rf = raft.Make(servers, me, persister, kv.applyCh)
	if maxraftstate < 0 {
		kv.rf.SetEnableSnapshot(false)
	}

	// You may need initialization code here.
	kv.store = make(map[string]string)
	kv.session.clientSessionMap = make(map[int]*clientSession)

	go kv.consumeApplyCh()

	return kv
}

func (kv *KVServer) HandleRequest(args *KVArgs, reply *KVReply) {
	// Your code here.

	logHeader := fmt.Sprintf("srv %d: ", kv.me)
	log.Printf("%s%s", logHeader, args.String())
	defer func() {
		log.Printf("%s%s", logHeader, reply.String())
	}()

	if !(args.Op == OP_GET || args.Op == OP_PUT || args.Op == OP_APPEND) {
		reply.Success = false
		reply.Msg = MSG_OP_UNSUPPORTED
		return
	}

	cs := kv.session.getClientSession(args.Cid)

	// validate xid and use cache
	lastAppliedXid, lastResp := cs.getLastAppliedXidAndResp()
	if lastAppliedXid > args.Xid {
		reply.Success = false
		reply.Msg = MSG_OLD_XID
		return
	} else if lastAppliedXid == args.Xid {
		*reply = lastResp
		return
	}

	//_, isLeader := kv.rf.GetState()

	//cs.mu.Lock()
	//// avoid append duplicate log entries of the same xid
	//if cs.lastSeenXid < args.Xid && isLeader {
	//	_, _, isLeader2 := kv.rf.Start(*args)
	//	isLeader = isLeader2
	//	if isLeader {
	//		cs.lastSeenXid = args.Xid
	//	}
	//}
	//cs.mu.Unlock()

	_, _, isLeader := kv.rf.Start(*args)

	if !isLeader {
		reply.Success = false
		reply.Msg = MSG_NOT_LEADER
		return
	}

	// wait until the request has been handled
	cs.condMu.Lock()
	for lastAppliedXid < args.Xid {
		cs.cond.Wait()
		lastAppliedXid, lastResp = cs.getLastAppliedXidAndResp()
	}
	cs.condMu.Unlock()

	if lastAppliedXid == args.Xid {
		*reply = lastResp
	} else {
		// a greater xid has been handled, suggesting that the client does not send one request at a time
		reply.Success = false
		reply.Msg = MSG_MULTIPLE_XID
	}
}

func (kv *KVServer) consumeApplyCh() {
	for applyMsg := range kv.applyCh {
		//log.Printf("srv %d: comsume applyMsg: %v", kv.me, applyMsg)
		if !applyMsg.CommandValid {
			// TODO: snapshot in 3B
			continue
		}

		args := applyMsg.Command.(KVArgs)
		cs := kv.session.getClientSession(args.Cid)
		lastXid, _ := cs.getLastAppliedXidAndResp()

		logHeader := fmt.Sprintf("srv %d: comsume applyMsg: ck %d: xid %d: ", kv.me, args.Cid, args.Xid)

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

			cs.setLastAppliedXidAndResp(args.Xid, reply)
			log.Printf("%shandle %s succeeds, key %q, value %q", logHeader, args.Op, args.Key, logV(reply.Value))
		} else if lastXid > args.Xid {
			log.Printf("%sWARN: lastAppliedXid %d > args.Xid %d", logHeader, lastXid, args.Xid)
		}

		cs.condMu.Lock()
		cs.cond.Broadcast()
		cs.condMu.Unlock()
	}
}
