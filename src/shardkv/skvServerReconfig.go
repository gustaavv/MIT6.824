package shardkv

import (
	"6.824/atopraft"
	"6.824/shardctrler"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	RECONFIG_STATUS_START   = 0
	RECONFIG_STATUS_PREPARE = 1
	RECONFIG_STATUS_COMMIT  = 2
)

func mapReConfigStatusToString(status int) string {
	switch status {
	case RECONFIG_STATUS_START:
		return "START"
	case RECONFIG_STATUS_PREPARE:
		return "PREPARE"
	case RECONFIG_STATUS_COMMIT:
		return "COMMIT"
	default:
		panic("invalid status")
	}
}

type ReConfigPayLoad struct {
	Status int
	// shard num -> shard data
	InData map[int]map[string]string
	// shard num
	OutShards []int
	Config    shardctrler.Config
}

func (pl ReConfigPayLoad) getInShards() []int {
	ans := make([]int, len(pl.InData))
	i := 0
	for k := range pl.InData {
		ans[i] = k
		i++
	}
	return ans
}

func (pl ReConfigPayLoad) String() string {
	return fmt.Sprintf("ReConfigPayLoad{Status:%s, InShards:%v, outShards:%v, Config:%s}",
		mapReConfigStatusToString(pl.Status), pl.getInShards(), pl.OutShards, pl.Config)
}

func (pl ReConfigPayLoad) Clone() atopraft.ArgsPayLoad {
	ans := new(ReConfigPayLoad)
	ans.Config = pl.Config.Clone()
	ans.Status = pl.Status
	return ans
}

func (kv *ShardKV) getNextQueryConfigAt() time.Time {
	return time.Now().Add(kv.skvConfig.SrvQueryConfigFrequency)
}

func (kv *ShardKV) queryConfigTicker() {
	for !kv.BaseServer.Killed() {
		time.Sleep(kv.skvConfig.BC.TickerFrequency)
		kv.BaseServer.Mu.Lock()
		if kv.lastQueryConfigAt.After(time.Now()) {
			kv.BaseServer.Mu.Unlock()
			continue
		}
		kv.BaseServer.Mu.Unlock()

		kv.queryConfigAndUpdate()

		kv.BaseServer.Mu.Lock()
		kv.lastQueryConfigAt = kv.getNextQueryConfigAt()
		kv.BaseServer.Mu.Unlock()
	}
}

func (kv *ShardKV) queryConfigAndUpdate() {
	// this function is too time-consuming compared with others,
	// so only allowing leader to perform reConfig
	if !kv.BaseServer.CheckLeader() {
		return
	}
	kv.BaseServer.Mu.Lock()
	oldCfg := kv.config
	// update the config one by one instead of going to the latest one directly
	nextNum := oldCfg.Num + 1
	if kv.reConfigStatus != RECONFIG_STATUS_COMMIT {
		kv.BaseServer.Mu.Unlock()
		return
	}
	kv.BaseServer.Mu.Unlock()

	newCfg := kv.scClerk.BaseQuery(nextNum, -1) // query only once

	if !(newCfg.Num > 0 && newCfg.Num == nextNum) {
		return
	}
	logHeader := fmt.Sprintf("%sSrv %d: group %d: reConfig: nextNum %d: ",
		kv.skvConfig.BC.LogPrefix, kv.BaseServer.Sid, kv.gid, nextNum)

	// only one goroutine can do the reConfig process
	kv.reConfigMu.Lock()
	defer kv.reConfigMu.Unlock()
	allGroups := merge2Groups(oldCfg.Groups, newCfg.Groups)

	// 1. make sure all groups achieve consensus that they are in the same config
	log.Printf("%swait for other groups to be at least (%d, COMMIT)", logHeader, oldCfg.Num)
	kv.WaitUntilReConfigConsensus(oldCfg.Num, RECONFIG_STATUS_COMMIT, allGroups)
	log.Printf("%sall other groups are at least (%d, COMMIT)", logHeader, oldCfg.Num)

	if !kv.BaseServer.CheckLeader() {
		return
	}
	kv.BaseServer.SetUnavailable() // drain all log entries unconsumed

	// 2. Start a new log entry to mark the start of the reConfig
	args := atopraft.SrvArgs{
		Cid: -1, Xid: kv.reConfigXidGenerator.NextUid(), Tid: -1, Op: "",
		PayLoad: ReConfigPayLoad{Config: newCfg, Status: RECONFIG_STATUS_START},
	}
	index, _, isLeader := kv.BaseServer.Rf.Start(args)
	if !isLeader {
		return
	}
	kv.BaseServer.WaitUntilEntryConsumed(index)
	log.Printf("%sSTART entry consumed", logHeader)

	// 3. Communicate with other groups to get/send shards
	inShards, outShards := makeShardReConfigInfo(oldCfg.Shards, newCfg.Shards, kv.gid)
	log.Printf("%sinShards: %v, outShards: %v", logHeader, inShards, outShards)
	// TODO: optimization: if both inShards and outShards are empty, we can commit directly without communication with other groups

	// only send inShards requests, not sending outShards proactively. let other groups request their inShards,
	// which are this group's outShards
	inData := kv.WaitUntilInSardData(inShards)

	// 4. prepare
	args = atopraft.SrvArgs{
		Cid: -1, Xid: kv.reConfigXidGenerator.NextUid(), Tid: -1, Op: "",
		PayLoad: ReConfigPayLoad{Config: newCfg, Status: RECONFIG_STATUS_PREPARE, InData: inData, OutShards: outShards},
	}
	index, _, isLeader = kv.BaseServer.Rf.Start(args)
	if !isLeader {
		return
	}
	kv.BaseServer.WaitUntilEntryConsumed(index)
	log.Printf("%sPREPARE entry consumed", logHeader)

	// if all other groups are prepared, then sending outShards succeeds.
	log.Printf("%swait for other groups to be at least (%d, PREPARE)", logHeader, nextNum)
	kv.WaitUntilReConfigConsensus(nextNum, RECONFIG_STATUS_PREPARE, allGroups)
	log.Printf("%sall other groups are at least (%d, PREPARE)", logHeader, nextNum)
	// 5. commit
	args = atopraft.SrvArgs{
		Cid: -1, Xid: kv.reConfigXidGenerator.NextUid(), Tid: -1, Op: "",
		PayLoad: ReConfigPayLoad{Config: newCfg, Status: RECONFIG_STATUS_COMMIT, InData: inData, OutShards: outShards},
	}
	index, _, isLeader = kv.BaseServer.Rf.Start(args)
	if !isLeader {
		return
	}
	kv.BaseServer.WaitUntilEntryConsumed(index)
	log.Printf("%sCOMMIT entry consumed", logHeader)

	kv.BaseServer.SetAvailable()
	log.Printf("%s reConfig succeeds", logHeader)
}

// WaitUntilReConfigConsensus other groups' (num, status) should >= parameters' (num, status)
func (kv *ShardKV) WaitUntilReConfigConsensus(num int, status int, allGroups map[int][]string) {
	var wg sync.WaitGroup
	wg.Add(len(allGroups) - 1) // exclude kv's group
	if _, ok := allGroups[kv.gid]; !ok {
		wg.Add(1)
	}

	for gid, servers := range allGroups {
		if gid == kv.gid {
			continue
		}
		servers := servers
		go func() {
			loop := true
			for loop {
				// TODO: use timeout and goroutines to do RPC
				for _, serverName := range servers {
					end := kv.make_end(serverName)
					args := new(ReConfigStatusArgs)
					args.Sid = kv.BaseServer.Sid
					reply := new(ReConfigStatusReply)
					b := end.Call("ShardKV.ReConfigStatus", args, reply)
					if !(b && reply.Success) {
						continue
					}
					// note that the value for START, PREPARE and COMMIT statuses are in strict ascending order,
					// so we can compare like this
					if (reply.Num >= num) || (reply.Num == num && reply.Status >= status) {
						loop = false
						wg.Done()
						break
					}
				}
				time.Sleep(kv.skvConfig.SrvRPCFrequency)
			}

		}()
	}
	wg.Wait()
}

type ReConfigStatusArgs struct {
	Sid int
}

func (args *ReConfigStatusArgs) String() string {
	return fmt.Sprintf("ReConfigStatusArgs{Sid:%d}", args.Sid)
}

type ReConfigStatusReply struct {
	Success bool
	Num     int
	Status  int
}

func (reply *ReConfigStatusReply) String() string {
	return fmt.Sprintf("ReConfigStatusReply{Success:%v, Num:%d, Status:%s}",
		reply.Success, reply.Num, mapReConfigStatusToString(reply.Status))
}

func (kv *ShardKV) ReConfigStatus(args *ReConfigStatusArgs, reply *ReConfigStatusReply) {
	kv.BaseServer.Mu.Lock()
	defer kv.BaseServer.Mu.Unlock()
	defer func() {
		log.Printf("%sSrv %d: ReConfigStatus: args %s, reply %s",
			kv.skvConfig.BC.LogPrefix, kv.BaseServer.Sid, args.String(), reply.String())
	}()
	if !kv.BaseServer.CheckLeader() {
		reply.Success = false
		return
	}

	reply.Success = true
	reply.Num = kv.reConfigNum
	reply.Status = kv.reConfigStatus
}

// makeShardReConfigInfo
// inShards are the shards this group needs in the new config
// outShards are the shards this group does not need in the new config
func makeShardReConfigInfo(
	oldShard [shardctrler.NShards]int,
	newShard [shardctrler.NShards]int,
	gid int) (inShards []int, outShards []int) {
	inShards = make([]int, 0)
	outShards = make([]int, 0)

	for i := 0; i < shardctrler.NShards; i++ {
		if oldShard[i] == gid && newShard[i] != gid {
			outShards = append(outShards, i)
		} else if oldShard[i] != gid && newShard[i] == gid {
			inShards = append(inShards, i)
		}
	}

	return
}

func merge2Groups(oldGroups map[int][]string, newGroups map[int][]string) map[int][]string {
	ans := make(map[int][]string)

	for gid, servers := range oldGroups {
		ans[gid] = make([]string, len(servers))
		copy(ans[gid], servers)
	}

	for gid, servers := range newGroups {
		if _, ok := oldGroups[gid]; !ok {
			ans[gid] = make([]string, len(servers))
			copy(ans[gid], servers)
		} else {
			for _, server := range servers {
				existed := false
				for _, oldServer := range oldGroups[gid] {
					if oldServer == server {
						existed = true
						break
					}
				}
				if !existed {
					ans[gid] = append(ans[gid], server)
				}
			}
		}
	}

	return ans
}

func (kv *ShardKV) WaitUntilInSardData(InShards []int) map[int]map[string]string {
	kv.BaseServer.Mu.Lock()
	configNum := kv.config.Num
	groups := kv.config.Groups
	shards := kv.config.Shards
	kv.BaseServer.Mu.Unlock()

	ans := make(map[int]map[string]string)
	var ansMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(InShards))

	for _, shard := range InShards {
		shard := shard
		go func() {
			if shards[shard] == 0 {
				ansMu.Lock()
				ans[shard] = make(map[string]string)
				ansMu.Unlock()
				wg.Done()
				return
			}

			servers := groups[shards[shard]]
			loop := true
			for loop {
				// TODO: use timeout and goroutines to do RPC
				for _, server := range servers {
					end := kv.make_end(server)
					args := new(GetShardDataArgs)
					args.Sid = kv.BaseServer.Sid
					args.ConfigNum = configNum
					args.Shard = shard
					reply := new(GetShardDataReply)
					b := end.Call("ShardKV.GetShardData", args, reply)

					if !(b && reply.Success) {
						continue
					}

					ansMu.Lock()
					ans[shard] = reply.Data
					ansMu.Unlock()
					loop = false
					wg.Done()
					break
				}
				time.Sleep(kv.skvConfig.SrvRPCFrequency)
			}
		}()
	}
	wg.Wait()
	return ans
}

type GetShardDataArgs struct {
	Sid       int
	ConfigNum int
	Shard     int
}

func (args *GetShardDataArgs) String() string {
	return fmt.Sprintf("GetShardDataArgs{Sid:%d, ConfigNum:%d, Shard:%d}",
		args.Sid, args.ConfigNum, args.Shard)
}

type GetShardDataReply struct {
	Success bool
	Data    map[string]string
}

func (reply *GetShardDataReply) String() string {
	return fmt.Sprintf("GetShardDataReply{Success:%v, Data:Len=%d}", reply.Success, len(reply.Data))
}

func (kv *ShardKV) GetShardData(args *GetShardDataArgs, reply *GetShardDataReply) {
	defer func() {
		log.Printf("%sSrv %d: GetShardData: args %s, reply %s",
			kv.skvConfig.BC.LogPrefix, kv.BaseServer.Sid, args.String(), reply.String())
	}()

	if !kv.BaseServer.CheckLeader() {
		reply.Success = false
		return
	}

	kv.BaseServer.Mu.Lock()
	defer kv.BaseServer.Mu.Unlock()

	if _, ok := kv.myShards[args.Shard]; !ok {
		reply.Success = false
		return
	}

	// TODO: validate args.ConfigNum?

	// We do not verify whether the requester should get data in this reConfig

	reply.Success = true
	reply.Data = kv.CloneShard(args.Shard)
}

func (kv *ShardKV) CloneShard(shard int) map[string]string {
	return cloneStr2StrMap(kv.BaseServer.Store.([]map[string]string)[shard])
}
