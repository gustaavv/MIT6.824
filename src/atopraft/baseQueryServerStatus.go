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
	ck.Mu.Lock()
	defer ck.Mu.Unlock()
	if ck.ServerStatus[i].Tid < tid {
		ck.ServerStatus[i].Tid = tid
		ck.ServerStatus[i].IsLeader = isLeader
	}
}

func (ck *BaseClerk) getPossibleLeaders() []int {
	ck.Mu.Lock()
	defer ck.Mu.Unlock()

	ans := make([]int, 0)

	for i, status := range ck.ServerStatus {
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
	ck.Mu.Lock()
	defer ck.Mu.Unlock()
	for i := range ck.ServerStatus {
		ck.ServerStatus[i].IsLeader = false
	}
}

func (ck *BaseClerk) querySingleServerStatus(index int) {
	server := ck.Servers[index]

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

	for i := range ck.Servers {
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

		ck.Mu.Lock()
		if ck.lastQueryServerStatusAt.After(time.Now()) {
			ck.Mu.Unlock()
			continue
		}

		ck.queryAllServerStatus()
		ck.lastQueryServerStatusAt = ck.getNextQueryServerStatusAt()
		ck.Mu.Unlock()
	}
}

//////////////////////////////////// server code /////////////////////////////////////////

func (srv *BaseServer) ServerStatus(args *ServerStatusArgs, reply *ServerStatusReply) {
	_, isLeader := srv.Rf.GetState()
	reply.IsLeader = isLeader
	reply.Tid = args.Tid
}
