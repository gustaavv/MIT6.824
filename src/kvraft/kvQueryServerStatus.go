package kvraft

import (
	"time"
)

type ServerStatusArgs struct {
	Cid int
	Tid int
}

type ServerStatusReply struct {
	IsLeader bool
	Term     int
	Tid      int
}

//////////////////////////////////// clerk code /////////////////////////////////////////

func (ck *Clerk) getPossibleLeaders() []int {
	ck.mu.Lock()
	defer ck.mu.Unlock()

	ans := make([]int, 0)

	// TODO: don't consider term now
	//for i, status := range ck.serverStatusArr {
	//	if status.Term > term && status.IsLeader {
	//		term = status.Term
	//		ans = make([]int, 0)
	//		ans = append(ans, i)
	//	} else if status.Term == term && status.IsLeader { // duplicate leader?
	//		ans = append(ans, i)
	//	}
	//}

	for i, status := range ck.serverStatusArr {
		if status.IsLeader {
			ans = append(ans, i)
		}
	}

	// if no possible leaders, send to all
	if len(ans) == 0 {
		ans = make([]int, len(ck.servers))
		for i := range ans {
			ans[i] = i
		}
	}

	return ans
}

func (ck *Clerk) querySingleServerStatus(index int) {
	server := ck.servers[index]

	args := new(ServerStatusArgs)
	args.Cid = ck.cid
	args.Tid = ck.tidGenerator.nextUid()

	reply := new(ServerStatusReply)
	server.Call("KVServer.ServerStatus", args, reply)

	ck.mu.Lock()
	defer ck.mu.Unlock()
	if ck.serverStatusArr[index].Tid < reply.Tid {
		ck.serverStatusArr[index] = reply
	}
}

func (ck *Clerk) queryAllServerStatus() {
	for i := range ck.servers {
		go ck.querySingleServerStatus(i)
	}
}

func getNextQueryServerStatusAt() time.Time {
	return time.Now().Add(QUERY_SERVER_STATUS_FREQUENCY)
}

func (ck *Clerk) queryServerStatusTicker() {
	// TODO: how will a clerk be killed?
	for {
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
	term, isLeader := kv.rf.GetState()
	reply.Term = term
	reply.IsLeader = isLeader
	reply.Tid = args.Tid
}
