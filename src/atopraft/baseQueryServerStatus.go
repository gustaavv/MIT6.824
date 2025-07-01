package atopraft

import (
	"fmt"
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

func (ck *BaseClerk) setServerStatus(i int, tid int, isLeader bool) {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	if ck.serverStatus[i].Tid < tid {
		ck.serverStatus[i].Tid = tid
		ck.serverStatus[i].IsLeader = isLeader
	}
}

func (ck *BaseClerk) getPossibleLeaders() []int {
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

func (ck *BaseClerk) resetPossibleLeaders() {
	ck.mu.Lock()
	defer ck.mu.Unlock()
	for i := range ck.serverStatus {
		ck.serverStatus[i].IsLeader = false
	}
}

func (ck *BaseClerk) querySingleServerStatus(index int) {
	server := ck.servers[index]

	args := new(ServerStatusArgs)
	args.Cid = ck.Cid
	args.Tid = ck.TidGenerator.NextUid()

	reply := new(ServerStatusReply)

	ok := server.Call(fmt.Sprintf("%s.ServerStatus", ck.RPCServerName), args, reply)
	if !ok {
		return
	}

	ck.setServerStatus(index, args.Tid, reply.IsLeader)
}

func (ck *BaseClerk) queryAllServerStatus() {
	if !ck.Config.EnableQueryServerStatus {
		return
	}

	for i := range ck.servers {
		go ck.querySingleServerStatus(i)
	}
}

func (ck *BaseClerk) getNextQueryServerStatusAt() time.Time {
	return time.Now().Add(ck.Config.QueryServerStatusFrequency)
}

func (ck *BaseClerk) queryServerStatusTicker() {

	if !ck.Config.EnableQueryServerStatus {
		return
	}

	for !ck.Killed() {
		time.Sleep(ck.Config.TickerFrequency)

		ck.mu.Lock()
		if ck.lastQueryServerStatusAt.After(time.Now()) {
			ck.mu.Unlock()
			continue
		}

		ck.queryAllServerStatus()
		ck.lastQueryServerStatusAt = ck.getNextQueryServerStatusAt()
		ck.mu.Unlock()
	}
}

//////////////////////////////////// server code /////////////////////////////////////////

func (srv *BaseServer) ServerStatus(args *ServerStatusArgs, reply *ServerStatusReply) {
	_, isLeader := srv.Rf.GetState()
	reply.IsLeader = isLeader
	reply.Tid = args.Tid
}
