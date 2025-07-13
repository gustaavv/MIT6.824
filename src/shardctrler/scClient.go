package shardctrler

//
// Shardctrler clerk.
//

import (
	"6.824/atopraft"
	"6.824/labrpc"
	"fmt"
	"log"
)

var ClerkIdGenerator atopraft.UidGenerator

type Clerk struct {
	BaseClerk *atopraft.BaseClerk
}

func (ck *Clerk) Kill() {
	ck.BaseClerk.Kill()
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	// Your code here.
	cid := ClerkIdGenerator.NextUid()
	ck.BaseClerk = atopraft.MakeBaseClerk(ck, cid, servers, makeSCConfig(), "ShardCtrler", handleFailureMsg)
	return ck
}

func MakeClerk2(servers []*labrpc.ClientEnd, config *atopraft.BaseConfig) *Clerk {
	if config == nil {
		config = makeSCConfig()
	}
	ck := new(Clerk)
	cid := ClerkIdGenerator.NextUid()
	ck.BaseClerk = atopraft.MakeBaseClerk(ck, cid, servers, config, "ShardCtrler", handleFailureMsg)
	return ck
}

func (ck *Clerk) Query(num int) Config {
	return ck.BaseQuery(num, 1)
}

func (ck *Clerk) BaseQuery(num int, count int) Config {
	xid := ck.BaseClerk.XidGenerator.NextUid()
	logHeader := fmt.Sprintf("%sCk %d: xid %d: ",
		ck.BaseClerk.Config.LogPrefix, ck.BaseClerk.Cid, xid)
	if ck.BaseClerk.Config.EnableLog {
		log.Printf("%sstart new query request, num %d", logHeader, num)
	}
	payload := SCPayLoad{Num: num}
	replyValue := ck.BaseClerk.DoRequest(&payload, OP_QUERY, xid, count)
	if replyValue == nil {
		return Config{Num: -1}
	}
	return replyValue.(SCReplyValue).Config
}

func (ck *Clerk) Join(servers map[int][]string) {
	xid := ck.BaseClerk.XidGenerator.NextUid()
	logHeader := fmt.Sprintf("%sCk %d: xid %d: ",
		ck.BaseClerk.Config.LogPrefix, ck.BaseClerk.Cid, xid)
	if ck.BaseClerk.Config.EnableLog {
		log.Printf("%sstart new join request, servers %v", logHeader, servers)
	}
	payload := SCPayLoad{Servers: servers}
	ck.BaseClerk.DoRequest(&payload, OP_JOIN, xid, 1)
}

func (ck *Clerk) Leave(gids []int) {
	xid := ck.BaseClerk.XidGenerator.NextUid()
	logHeader := fmt.Sprintf("%sCk %d: xid %d: ",
		ck.BaseClerk.Config.LogPrefix, ck.BaseClerk.Cid, xid)
	if ck.BaseClerk.Config.EnableLog {
		log.Printf("%sstart new leave request, gids %v", logHeader, gids)
	}
	payload := SCPayLoad{GIDs: gids}
	ck.BaseClerk.DoRequest(&payload, OP_LEAVE, xid, 1)
}

func (ck *Clerk) Move(shard int, gid int) {
	xid := ck.BaseClerk.XidGenerator.NextUid()
	logHeader := fmt.Sprintf("%sCk %d: xid %d: ",
		ck.BaseClerk.Config.LogPrefix, ck.BaseClerk.Cid, xid)
	if ck.BaseClerk.Config.EnableLog {
		log.Printf("%sstart new move request, shard %d, gid %d", logHeader, shard, gid)
	}
	payload := SCPayLoad{Shard: shard, GID: gid}
	ck.BaseClerk.DoRequest(&payload, OP_MOVE, xid, 1)
}

func handleFailureMsg(ck *atopraft.BaseClerk, msg string, logHeader string) {}
