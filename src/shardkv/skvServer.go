package shardkv

import (
	"6.824/atopraft"
	"6.824/labrpc"
	"6.824/shardctrler"
	"log"
	"sync"
	"time"
)
import "6.824/raft"
import "6.824/labgob"

type SKVStore struct {
	Data [shardctrler.NShards]map[string]string
	// this field is used to check client requests, not for checking server requests
	MyShards       map[int]bool
	ReConfigStatus int
	ReConfigNum    int
	Config         shardctrler.Config
	SkvUnavailable bool
}

type ShardKV struct {
	BaseServer *atopraft.BaseServer
	rf         *raft.Raft
	make_end   func(string) *labrpc.ClientEnd
	gid        int

	skvConfig         *SKVConfig
	lastQueryConfigAt time.Time
	scClerk           *shardctrler.Clerk
	reConfigMu        sync.Mutex
	initLogConsumed   bool
}

func (kv *ShardKV) getReConfigTuple() (reConfigNum int, reConfigStatus int) {
	kv.BaseServer.Mu.Lock()
	defer kv.BaseServer.Mu.Unlock()
	reConfigNum = kv.BaseServer.Store.(SKVStore).ReConfigNum
	reConfigStatus = kv.BaseServer.Store.(SKVStore).ReConfigStatus
	return
}

// Kill
// the tester calls Kill() when a ShardKV instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (kv *ShardKV) Kill() {
	kv.BaseServer.Kill()
}

var allowedOps = []string{OP_GET, OP_PUT, OP_APPEND}

var serverIdGenerator atopraft.UidGenerator

// StartServer
// servers[] contains the ports of the servers in this group.
//
// me is the index of the current server in servers[].
//
// the k/v server should store snapshots through the underlying Raft
// implementation, which should call persister.SaveStateAndSnapshot() to
// atomically save the Raft state along with the snapshot.
//
// the k/v server should snapshot when Raft's saved state exceeds
// maxraftstate bytes, in order to allow Raft to garbage-collect its
// log. if maxraftstate is -1, you don't need to snapshot.
//
// gid is this group's GID, for interacting with the shardctrler.
//
// pass ctrlers[] to shardctrler.MakeClerk() so you can send
// RPCs to the shardctrler.
//
// make_end(servername) turns a server name from a
// Config.Groups[gid][i] into a labrpc.ClientEnd on which you can
// send RPCs. You'll need this to send RPCs to other groups.
//
// look at client.go for examples of how to use ctrlers[]
// and make_end() to send RPCs to the group owning a specific shard.
//
// StartServer() must return quickly, so it should start goroutines
// for any long-running work.
//
func StartServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int, gid int, ctrlers []*labrpc.ClientEnd, make_end func(string) *labrpc.ClientEnd) *ShardKV {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(SKVPayLoad{})
	labgob.Register(SKVReplyValue(""))
	labgob.Register(ReConfigPayLoad{})
	labgob.Register(ReConfigStatusArgs{})
	labgob.Register(ReConfigStatusReply{})
	labgob.Register(GetShardDataArgs{})
	labgob.Register(GetShardDataReply{})

	kv := new(ShardKV)
	sid := serverIdGenerator.NextUid()
	kv.BaseServer = atopraft.StartBaseServer(sid, kv, servers, me, persister, maxraftstate, allowedOps,
		businessLogic, buildStore, decodeStore, validateRequest, makeSKVConfig().BC)
	kv.rf = kv.BaseServer.Rf
	kv.make_end = make_end
	kv.gid = gid

	kv.scClerk = shardctrler.MakeClerk2(ctrlers, nil)
	kv.skvConfig = makeSKVConfig()

	go kv.queryConfigTicker()

	return kv
}

func businessLogic(srv *atopraft.BaseServer, args atopraft.SrvArgs, reply *atopraft.SrvReply, logHeader string) {
	store := srv.Store.(SKVStore)
	skv := srv.AtopSrv.(*ShardKV)

	switch args.PayLoad.(type) {
	case SKVPayLoad:
		if store.SkvUnavailable {
			reply.Success = false
			reply.Msg = atopraft.MSG_UNAVAILABLE
			return
		}
		payload := args.PayLoad.(SKVPayLoad)
		store2 := store.Data
		shard := key2shard(payload.Key)

		if _, ok := store.MyShards[shard]; !ok {
			reply.Success = false
			reply.Msg = MSG_NOT_MY_SHARD
			return
		}

		switch args.Op {
		case OP_GET:
			v, ok := store2[shard][payload.Key]
			if !ok {
				v = ""
			}
			v2 := SKVReplyValue(v)
			reply.Value = &v2
		case OP_PUT:
			store2[shard][payload.Key] = payload.Value
			v2 := SKVReplyValue("")
			reply.Value = &v2
		case OP_APPEND:
			v, ok := store2[shard][payload.Key]
			if !ok {
				v = ""
			}
			v += payload.Value
			store2[shard][payload.Key] = v
			v2 := SKVReplyValue("")
			if skv.skvConfig.returnValueForAppend {
				v2 = SKVReplyValue(v)
			}
			reply.Value = &v2
		}
	case ReConfigPayLoad:
		payload := args.PayLoad.(ReConfigPayLoad)

		if store.ReConfigStatus == RECONFIG_STATUS_COMMIT &&
			payload.Status == RECONFIG_STATUS_START &&
			payload.Config.Num == store.ReConfigNum+1 {
			// new config
			store.ReConfigNum++
			store.ReConfigStatus = RECONFIG_STATUS_START
			for _, shardNum := range payload.OutShards {
				if _, ok := store.MyShards[shardNum]; !ok {
					log.Fatalf("")
				}
				delete(store.MyShards, shardNum)
			}
			log.Printf("%sgroup %d: reConfig: (%d, START)", logHeader, skv.gid, store.ReConfigNum)
		} else if store.ReConfigStatus == RECONFIG_STATUS_START &&
			payload.Status == RECONFIG_STATUS_PREPARE &&
			payload.Config.Num == store.ReConfigNum {
			// apply inData (adding shards) when preparing
			for shardNum, shardDataReply := range payload.InData {
				if _, ok := store.MyShards[shardNum]; ok {
					log.Fatalf("")
				}
				store.Data[shardNum] = atopraft.CloneStr2StrMap(shardDataReply.Data)
				store.MyShards[shardNum] = true
				srv.Session.Update(shardDataReply.ClientSessionList)
			}
			store.ReConfigStatus = RECONFIG_STATUS_PREPARE
			log.Printf("%sgroup %d: reConfig: (%d, PREPARE)", logHeader, skv.gid, store.ReConfigNum)
		} else if store.ReConfigStatus == RECONFIG_STATUS_PREPARE &&
			payload.Status == RECONFIG_STATUS_COMMIT &&
			payload.Config.Num == store.ReConfigNum {
			// delete shards when commiting
			for _, shardNum := range payload.OutShards {
				store.Data[shardNum] = make(map[string]string)
			}
			store.Config = payload.Config
			store.ReConfigStatus = RECONFIG_STATUS_COMMIT
			log.Printf("%sgroup %d: reConfig: (%d, COMMIT)", logHeader, skv.gid, store.ReConfigNum)
		} else {
			// TODO log warning?
		}

	}

	srv.Store = store
}

func buildStore() interface{} {
	ans := SKVStore{}

	for i := 0; i < shardctrler.NShards; i++ {
		ans.Data[i] = make(map[string]string)
	}

	ans.MyShards = make(map[int]bool)
	ans.ReConfigNum = 0
	ans.ReConfigStatus = RECONFIG_STATUS_COMMIT

	return ans
}

func decodeStore(d *labgob.LabDecoder) (interface{}, error) {
	var store SKVStore
	err := d.Decode(&store)
	return store, err
}

func validateRequest(srv *atopraft.BaseServer, args *atopraft.SrvArgs, reply *atopraft.SrvReply) bool {
	srv.Mu.Lock()
	defer srv.Mu.Unlock()

	payload := args.PayLoad.(SKVPayLoad)

	if _, ok := srv.Store.(SKVStore).MyShards[key2shard(payload.Key)]; !ok {
		reply.Success = false
		reply.Msg = MSG_NOT_MY_SHARD
		return false
	}

	return true
}

func (kv *ShardKV) HandleRequest(args *atopraft.SrvArgs, reply *atopraft.SrvReply) {
	kv.BaseServer.HandleRequest(args, reply)
}

func (kv *ShardKV) ServerStatus(args *atopraft.ServerStatusArgs, reply *atopraft.ServerStatusReply) {
	kv.BaseServer.ServerStatus(args, reply)
}
