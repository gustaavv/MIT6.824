package shardkv

import (
	"6.824/atopraft"
	"6.824/labrpc"
	"6.824/shardctrler"
	"log"
	"sync/atomic"
	"time"
)
import "6.824/raft"
import "6.824/labgob"

type ShardKV struct {
	BaseServer *atopraft.BaseServer
	rf         *raft.Raft
	make_end   func(string) *labrpc.ClientEnd
	gid        int

	// store fields

	myShards       map[int]bool
	reConfigStatus int
	reConfigNum    int

	config               shardctrler.Config
	skvUnavailable       int32
	skvConfig            *SKVConfig
	lastQueryConfigAt    time.Time
	scClerk              *shardctrler.Clerk
	ReConfigXidGenerator atopraft.UidGenerator
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

func (kv *ShardKV) setSKVUnavailable() {
	atomic.StoreInt32(&kv.skvUnavailable, 1)
}

func (kv *ShardKV) setSKVAvailable() {
	atomic.StoreInt32(&kv.skvUnavailable, 0)
}

func (kv *ShardKV) checkSKVAvailable() bool {
	return atomic.LoadInt32(&kv.skvUnavailable) == 0
}

var allowedOps = []string{OP_GET, OP_PUT, OP_APPEND}

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
	kv.BaseServer = atopraft.StartBaseServer(kv, servers, me, persister, maxraftstate, allowedOps,
		businessLogic, buildStore, decodeStore, validateRequest, makeSKVConfig().BC)
	kv.rf = kv.BaseServer.Rf
	kv.make_end = make_end
	kv.gid = gid

	kv.myShards = make(map[int]bool)
	kv.reConfigNum = 0
	kv.reConfigStatus = RECONFIG_STATUS_COMMIT

	kv.scClerk = shardctrler.MakeClerk2(ctrlers, nil)
	kv.skvConfig = makeSKVConfig()

	go kv.queryConfigTicker()

	return kv
}

func (kv *ShardKV) CloneShard(shard int) map[string]string {
	ans := make(map[string]string)
	for k, v := range kv.BaseServer.Store.([]map[string]string)[shard] {
		ans[k] = v
	}

	return ans
}

func businessLogic(srv *atopraft.BaseServer, args atopraft.SrvArgs, reply *atopraft.SrvReply, logHeader string) {
	skv := srv.AtopSrv.(*ShardKV)

	switch args.PayLoad.(type) {
	case SKVPayLoad:
		if !skv.checkSKVAvailable() {
			reply.Success = false
			reply.Msg = atopraft.MSG_UNAVAILABLE
			return
		}
		payload := args.PayLoad.(SKVPayLoad)
		store2 := srv.Store.([]map[string]string)
		shard := key2shard(payload.Key)
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
			store2[shard][payload.Key] = v + payload.Value
			v2 := SKVReplyValue("")
			reply.Value = &v2
		}
	case ReConfigPayLoad:
		payload := args.PayLoad.(ReConfigPayLoad)

		if skv.reConfigStatus == RECONFIG_STATUS_COMMIT &&
			payload.Status == RECONFIG_STATUS_START &&
			payload.Config.Num == skv.reConfigNum+1 {
			// new config
			skv.reConfigNum++
			skv.reConfigStatus = RECONFIG_STATUS_START
			skv.setSKVUnavailable()
			log.Printf("%sgroup %d: reConfig: (%d, START)", logHeader, skv.gid, skv.reConfigNum)
		} else if skv.reConfigStatus == RECONFIG_STATUS_START &&
			payload.Status == RECONFIG_STATUS_PREPARE &&
			payload.Config.Num == skv.reConfigNum {
			// apply inData (adding shards) when preparing
			store2 := srv.Store.([]map[string]string)
			for shardNum, shardData := range payload.InData {
				if _, ok := skv.myShards[shardNum]; ok {
					log.Fatalf("")
				}
				store2[shardNum] = shardData
				skv.myShards[shardNum] = true
			}
			skv.reConfigStatus = RECONFIG_STATUS_PREPARE
			log.Printf("%sgroup %d: reConfig: (%d, PREPARE)", logHeader, skv.gid, skv.reConfigNum)
		} else if skv.reConfigStatus == RECONFIG_STATUS_PREPARE &&
			payload.Status == RECONFIG_STATUS_COMMIT &&
			payload.Config.Num == skv.reConfigNum {
			// apply outData (delete shards) when commiting
			for _, shardNum := range payload.OutData {
				if _, ok := skv.myShards[shardNum]; !ok {
					log.Fatalf("")
				}
				store2 := srv.Store.([]map[string]string)
				store2[shardNum] = make(map[string]string)
			}
			skv.config = payload.Config
			skv.reConfigStatus = RECONFIG_STATUS_COMMIT
			skv.setSKVAvailable()
			log.Printf("%sgroup %d: reConfig: (%d, COMMIT)", logHeader, skv.gid, skv.reConfigNum)
		} else {
			// TODO log warning?
		}

	}
}

func buildStore() interface{} {
	// TODO may need to define a store struct to include multiple fields

	// shard the kv store by key at the beginning
	ans := make([]map[string]string, shardctrler.NShards)
	for i := 0; i < shardctrler.NShards; i++ {
		ans[i] = make(map[string]string)
	}
	return ans
}

func decodeStore(d *labgob.LabDecoder) (interface{}, error) {
	var store []map[string]string
	err := d.Decode(&store)
	return store, err
}

func validateRequest(srv *atopraft.BaseServer, args *atopraft.SrvArgs, reply *atopraft.SrvReply) bool {
	srv.Mu.Lock()
	defer srv.Mu.Unlock()

	payload := args.PayLoad.(SKVPayLoad)
	atopSrv := srv.AtopSrv.(*ShardKV)

	//if payload.ConfigNum > atopSrv.config.Num {
	//	go func() {
	//		srv.Mu.Lock()
	//		atopSrv.queryConfigAndUpdate()
	//		srv.Mu.Unlock()
	//	}()
	//}

	if payload.ConfigNum != atopSrv.config.Num {
		reply.Success = false
		reply.Msg = MSG_CONFIG_NUM_MISMATCH
		return false
	}

	if _, ok := atopSrv.myShards[key2shard(payload.Key)]; !ok {
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
