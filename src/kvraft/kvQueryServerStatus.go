package kvraft

import (
	"time"
)

type ServerStatusArgs struct {
	Cid int
	Tid int
}

type ServerStatusReply struct {
	Tid      int
	IsLeader bool
}

//////////////////////////////////// clerk code /////////////////////////////////////////

func (ck *Clerk) setServerStatus(i int, tid int, isLeader bool) {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	if ck.serverStatus[i].Tid < tid {
		ck.serverStatus[i].Tid = tid
		ck.serverStatus[i].IsLeader = isLeader
	}
}

func (ck *Clerk) getPossibleLeaders() []int {
	ck.mu.Lock()
	defer ck.mu.Unlock()

	ans := make([]int, 0)

	for i, status := range ck.serverStatus {
		if status.IsLeader {
			ans = append(ans, i)
		}
	}

	// if no possible leaders, send to all
	//if len(ans) == 0 {
	//	ans = ck.allLeaderIndex
	//}

	return ans
}

func (ck *Clerk) resetPossibleLeaders() {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	for i := range ck.serverStatus {
		ck.serverStatus[i].IsLeader = false
	}
}

func (ck *Clerk) querySingleServerStatus(index int) {
	server := ck.servers[index]

	args := new(ServerStatusArgs)
	args.Cid = ck.cid
	args.Tid = ck.tidGenerator.nextUid()

	reply := new(ServerStatusReply)
	ok := server.Call("KVServer.ServerStatus", args, reply)
	if !ok {
		return
	}

	ck.setServerStatus(index, args.Tid, reply.IsLeader)
}

func (ck *Clerk) queryAllServerStatus() {
	if !ENABLE_QUERY_SERVER_STATUS {
		return
	}

	for i := range ck.servers {
		go ck.querySingleServerStatus(i)
	}
}

func getNextQueryServerStatusAt() time.Time {
	return time.Now().Add(QUERY_SERVER_STATUS_FREQUENCY)
}

func (ck *Clerk) queryServerStatusTicker() {

	if !ENABLE_QUERY_SERVER_STATUS {
		return
	}

	for !ck.killed() {
		time.Sleep(TICKER_FREQUENCY)

		ck.mu.Lock()
		if ck.lastQueryServerStatusAt.After(time.Now()) {
			ck.mu.Unlock()
			continue
		}

		ck.queryAllServerStatus()
		ck.lastQueryServerStatusAt = getNextQueryServerStatusAt()
		ck.mu.Unlock()
	}
}

//////////////////////////////////// server code /////////////////////////////////////////

func (kv *KVServer) ServerStatus(args *ServerStatusArgs, reply *ServerStatusReply) {
	_, isLeader := kv.rf.GetState()
	reply.IsLeader = isLeader
	reply.Tid = args.Tid
}
