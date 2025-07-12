package atopraft

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"6.824/labrpc"
)

type BaseClerk struct {
	Servers []*labrpc.ClientEnd
	// You will have to modify this struct.

	AtopCk interface{}
	Mu     sync.Mutex
	dead   int32 // set by Kill()

	Cid           int
	Config        *BaseConfig
	RPCServerName string

	// last completed operation
	lastXid int
	// xid -> SrvReply.Value
	respCache map[int]ReplyValue

	TidGenerator UidGenerator
	XidGenerator UidGenerator

	ServerStatus            []*ServerStatusReply
	lastQueryServerStatusAt time.Time
	allLeaderIndex          []int // see getPossibleLeaders

	lastTrimCacheAt time.Time

	handleFailureMsg handleFailureMsg
}

type handleFailureMsg func(ck *BaseClerk, msg string, logHeader string)

func (ck *BaseClerk) Kill() {
	atomic.StoreInt32(&ck.dead, 1)
	log.Printf("%sCk %d: shutting down...", ck.Config.LogPrefix, ck.Cid)
}

func (ck *BaseClerk) Killed() bool {
	z := atomic.LoadInt32(&ck.dead)
	return z == 1
}

func MakeBaseClerk(atopCk interface{}, cid int, servers []*labrpc.ClientEnd, config *BaseConfig, rpcServerName string, handleFailureMsg handleFailureMsg) *BaseClerk {
	ck := new(BaseClerk)
	ck.Servers = servers
	ck.AtopCk = atopCk
	ck.Cid = cid
	ck.Config = config
	ck.RPCServerName = rpcServerName

	ck.respCache = make(map[int]ReplyValue)

	ck.ServerStatus = make([]*ServerStatusReply, len(servers))
	for i := 0; i < len(servers); i++ {
		ck.ServerStatus[i] = &ServerStatusReply{}
	}
	ck.lastQueryServerStatusAt = time.Now()

	ck.allLeaderIndex = make([]int, len(servers))
	for i := 0; i < len(servers); i++ {
		ck.allLeaderIndex[i] = i
	}

	ck.lastTrimCacheAt = ck.getNextTrimCacheAt()

	ck.handleFailureMsg = handleFailureMsg

	go ck.queryAllServerStatus()

	go ck.queryServerStatusTicker()
	go ck.TrimCacheTicker()

	log.Printf("%sCk %d: start", ck.Config.LogPrefix, ck.Cid)

	return ck
}

func (ck *BaseClerk) DoRequest(payload ArgsPayLoad, op string, xid int, count int) ReplyValue {
	if ck.Killed() {
		return nil
	}

	logHeader := fmt.Sprintf("%sCk %d: xid %d: count %d: ", ck.Config.LogPrefix, ck.Cid, xid, count)

	if lastXid := ck.getLastXid(); xid <= lastXid {
		resp, existed := ck.getRespCache(xid)
		if existed {
			return resp
		} else {
			log.Fatalf("%sresp not existed in cache (lastXid %d, maybe the cache item is deleted)", logHeader, lastXid)
		}
	}

	replyCh := make(chan ReplyValue)
	timeout := time.After(ck.Config.RequestTimeout)

	leaders := ck.getPossibleLeaders()
	log.Printf("%spossible leaders: %v", logHeader, leaders)
	for _, i := range leaders {
		i := i
		server := ck.Servers[i]
		go func() {
			args := new(SrvArgs)
			args.PayLoad = payload
			args.Op = op
			args.Cid = ck.Cid
			args.Xid = xid
			args.Tid = ck.TidGenerator.NextUid()

			reply := new(SrvReply)
			ok := server.Call(fmt.Sprintf("%s.HandleRequest", ck.RPCServerName), args, reply)

			logHeader := fmt.Sprintf("%sCk %d: xid %d: tid %d: count %d: srv %d: ",
				ck.Config.LogPrefix, ck.Cid, xid, args.Tid, count, i)

			if !ok {
				return
			}

			if reply.Success {
				ck.setServerStatus(i, args.Tid, true)
				var replyValue ReplyValue = nil
				if reply.Value != nil {
					replyValue = reply.Value.(ReplyValue)
				}
				ck.setRespCache(xid, replyValue)
				ck.setLastXid(xid)
				replyCh <- replyValue
				logValue := "nil"
				if replyValue != nil {
					logValue = replyValue.String()
				}
				log.Printf("%s%s succeeds, payload %s, value %s",
					logHeader, op, payload.String(), logValue)
			} else {
				switch reply.Msg {
				case MSG_NOT_LEADER:
					ck.setServerStatus(i, args.Tid, false)
				case MSG_OLD_XID:
					log.Printf("%swarn: send request with old xid to leader", logHeader)
					ck.setServerStatus(i, args.Tid, true)
				case MSG_MULTIPLE_XID:
					log.Printf("%swarn: wrong use of client, you should send requests with one xid at a time", logHeader)
					ck.setServerStatus(i, args.Tid, true)
				case MSG_OP_UNSUPPORTED:
					log.Fatalf("%sunsupported operation %s", logHeader, args.Op)
				case MSG_SHUTDOWN:
					log.Printf("%sserver is shutting down", logHeader)
					ck.setServerStatus(i, args.Tid, false)
				case MSG_READ_SNAPSHOT:
					log.Printf("%sserver is reading snapshot", logHeader)
				case MSG_UNAVAILABLE:
					log.Printf("%sserver is unavailable", logHeader)
				case MSG_INVALID_PAYLOAD:
					log.Fatalf("%sinvalid payload %s", logHeader, payload.String())
				case MSG_INVALID_ARGS:
					log.Fatalf("%sinvalid args %s", logHeader, args.String())
				default:
					ck.handleFailureMsg(ck, reply.Msg, logHeader)
				}
			}
		}()
	}

	select {
	case v := <-replyCh:
		return v
	case <-timeout:
		log.Printf("%stimeout, resend requests", logHeader)
		//ck.resetPossibleLeaders()
		if count < 0 {
			return nil
		}
		return ck.DoRequest(payload, op, xid, count+1) // TODO: set max retries?
	}
}
