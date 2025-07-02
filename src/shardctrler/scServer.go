package shardctrler

import (
	"6.824/atopraft"
	"6.824/raft"
	"log"
	"sort"
)
import "6.824/labrpc"
import "6.824/labgob"

type ShardCtrler struct {
	BaseServer *atopraft.BaseServer
	rf         *raft.Raft
}

// Kill
// the tester calls Kill() when a ShardCtrler instance won't
// be needed again. you are not required to do anything
// in Kill(), but it might be convenient to (for example)
// turn off debug output from this instance.
//
func (sc *ShardCtrler) Kill() {
	sc.BaseServer.Kill()
}

// Raft needed by shardkv tester
func (sc *ShardCtrler) Raft() *raft.Raft {
	return sc.BaseServer.Rf
}

var allowedOps = []string{OP_JOIN, OP_LEAVE, OP_MOVE, OP_QUERY}

// StartServer
// servers[] contains the ports of the set of
// servers that will cooperate via Raft to
// form the fault-tolerant shardctrler service.
// me is the index of the current server in servers[].
//
func StartServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister) *ShardCtrler {
	labgob.Register(SCPayLoad{})
	labgob.Register(SCReplyValue{})
	labgob.Register(Config{})

	sc := new(ShardCtrler)
	sc.BaseServer = atopraft.StartBaseServer(servers, me, persister, -1, allowedOps,
		businessLogic, buildStore, decodeStore, makeSCConfig())
	sc.rf = sc.BaseServer.Rf
	return sc
}

func businessLogic(srv *atopraft.BaseServer, args atopraft.SrvArgs, reply *atopraft.SrvReply) {
	store := srv.Store.([]Config)
	payload := args.PayLoad.(SCPayLoad)

	// query requests
	// do not forget to return early
	switch args.Op {
	case OP_QUERY:
		index := payload.Num
		if !(0 <= index && index < len(store)) {
			index = len(store) - 1
		}
		rv := SCReplyValue{
			Config: store[index].Clone(),
		}
		reply.Value = &rv
		return
	}

	// reminder: do you make sure query requests return early?

	// update requests
	latestConfig := &store[len(store)-1]
	newConfig := (*latestConfig).Clone()
	newConfig.Num = latestConfig.Num + 1

	switch args.Op {
	case OP_JOIN:
		// 1. find groups to add
		_gidsToAdd := make(map[int]bool)
		for gid := range payload.Servers {
			if _, ok := latestConfig.Groups[gid]; !ok {
				_gidsToAdd[gid] = true
			} else {
				// clerk sends an existing gid to add, ignore this error for now
			}
		}
		// map traversal may be not deterministic, so we use this sorting method
		gidsToAdd := make([]int, 0)
		for gid := range _gidsToAdd {
			gidsToAdd = append(gidsToAdd, gid)
		}
		sort.Ints(gidsToAdd)

		if len(latestConfig.Groups) > 0 {
			// 2. get current assignment info
			shardAssignMap := make(map[int]int)
			for _, gid := range latestConfig.Shards {
				shardAssignMap[gid]++
			}
			// map traversal may be not deterministic, so we use this sorting method
			arr := make([][]int, 0) // entry: [count, gid]
			for gid, count := range shardAssignMap {
				arr = append(arr, []int{count, gid})
			}
			// order by count desc, gid asc
			sort.Slice(arr, func(i, j int) bool {
				if arr[i][0] != arr[j][0] {
					return arr[i][0] > arr[j][0]
				}
				return arr[i][1] < arr[j][1]
			})

			// 3. reassign
			newGidCount := len(gidsToAdd) + len(latestConfig.Groups)
			newShardPerGid := NShards / newGidCount // every new group needs this number of shards
			if newShardPerGid == 0 {                // the number of groups is greater than NShards
				newShardPerGid = 1
			}

			// the number of groups may be greater than NShards, which will not be assigned shards
			newGidToAssign := make([]int, 0)
			j := 0
			for i := len(latestConfig.Groups); i < NShards && j < len(gidsToAdd); i++ {
				newGidToAssign = append(newGidToAssign, gidsToAdd[j])
				j++
			}

			j = 0               // j points to arr
			curMax := arr[j][0] // curMax is the current maximum value of arr[i][0] for all i
			for _, newGid := range newGidToAssign {
				for i := 0; i < newShardPerGid; {
					if arr[j][0] != curMax { // always take a shard from the group that has the most shards
						j = 0
						curMax = arr[j][0]
					} else {
						oldGid := arr[j][1]
						// take a shard from oldGid
						flag := false
						for k, gid := range newConfig.Shards {
							if oldGid == gid {
								flag = true
								newConfig.Shards[k] = newGid
								arr[j][0]--
								break
							}
						}
						if !flag {
							log.Fatalf("")
						}
						i++
						j = (j + 1) % len(arr)
					}
				}
			}
		} else {
			// 3. assign
			// gid 0 should not be treated as a valid group during reassignment,
			// so we need this separate assign logic
			j := 0
			for i := 0; i < NShards; i++ {
				newConfig.Shards[i] = gidsToAdd[j]
				j = (j + 1) % len(gidsToAdd)
			}
		}

		// 4. add groups. Here we do not use gidsToAdd, because the instances may be changed for one group.
		for gid, instances := range payload.Servers {
			newConfig.Groups[gid] = instances
		}
	case OP_LEAVE:
		// 1. find groups to delete
		gidsToDelete := make(map[int]bool)
		for _, gid := range payload.GIDs {
			if _, ok := latestConfig.Groups[gid]; ok {
				gidsToDelete[gid] = true
			} else {
				// clerk sends an unexisted gid to delete, ignore this error for now
			}
		}

		// 2. find new shards to reassign and get current assignment info
		shardsToReassign := make([]int, 0)
		shardAssignMap := make(map[int]int)
		for gid := range latestConfig.Groups {
			if _, ok := gidsToDelete[gid]; !ok {
				shardAssignMap[gid] = 0
			}
		}
		for i, gid := range latestConfig.Shards {
			if _, ok := gidsToDelete[gid]; ok {
				shardsToReassign = append(shardsToReassign, i)
			} else {
				shardAssignMap[gid]++
			}
		}

		// 3. reassignment
		if len(shardAssignMap) > 0 {
			for _, newShard := range shardsToReassign {
				// map traversal may be not deterministic, so we use this sorting method
				arr := make([][]int, 0) // entry: [count, gid]
				for gid, count := range shardAssignMap {
					arr = append(arr, []int{count, gid})
				}
				// order by count asc, gid asc
				sort.Slice(arr, func(i, j int) bool {
					if arr[i][0] != arr[j][0] {
						return arr[i][0] < arr[j][0]
					}
					return arr[i][1] < arr[j][1]
				})

				// assign the new shard to the group with currently the least shards and the smallest gid
				newConfig.Shards[newShard] = arr[0][1]
				shardAssignMap[arr[0][1]]++
			}
		} else { // all groups deleted
			for i := 0; i < NShards; i++ {
				newConfig.Shards[i] = 0
			}
		}

		// 4. delete groups
		for gid := range gidsToDelete {
			delete(newConfig.Groups, gid)
		}

	case OP_MOVE:
		shard := payload.Shard
		gid := payload.GID

		// validate payload
		if !(0 <= shard && shard < NShards) {
			reply.Success = false
			reply.Msg = atopraft.MSG_INVALID_PAYLOAD
			return
		}

		if _, ok := latestConfig.Groups[gid]; !ok {
			reply.Success = false
			reply.Msg = atopraft.MSG_INVALID_PAYLOAD
			return
		}

		newConfig.Shards[shard] = gid
	}

	store = append(store, newConfig)
	if len(store) != newConfig.Num+1 {
		log.Fatalf("")
	}
	srv.Store = store
	log.Printf("%sSrv %d: req(ck %d, xid %d) leads to new config: %s",
		srv.Config.LogPrefix, srv.Me, args.Cid, args.Xid, newConfig.String())
}

func buildStore() interface{} {
	ans := make([]Config, 1)
	ans[0].Groups = map[int][]string{}
	return ans
}

func decodeStore(d *labgob.LabDecoder) (interface{}, error) {
	var store []Config
	err := d.Decode(&store)
	return store, err
}

func (sc *ShardCtrler) HandleRequest(args *atopraft.SrvArgs, reply *atopraft.SrvReply) {
	sc.BaseServer.HandleRequest(args, reply)
}

func (sc *ShardCtrler) ServerStatus(args *atopraft.ServerStatusArgs, reply *atopraft.ServerStatusReply) {
	sc.BaseServer.ServerStatus(args, reply)
}
