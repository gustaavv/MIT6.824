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

var clerkIdGenerator atopraft.UidGenerator

type Clerk struct {
	BaseClerk *atopraft.BaseClerk
}

func (ck *Clerk) Kill() {
	ck.BaseClerk.Kill()
}

func MakeClerk(servers []*labrpc.ClientEnd) *Clerk {
	ck := new(Clerk)
	// Your code here.
	cid := clerkIdGenerator.NextUid()
	ck.BaseClerk = atopraft.MakeBaseClerk(cid, servers, makeSCConfig(), "ShardCtrler")
	return ck
}

func (ck *Clerk) Query(num int) Config {
	xid := ck.BaseClerk.XidGenerator.NextUid()
	logHeader := fmt.Sprintf("%sCk %d: xid %d: ",
		ck.BaseClerk.Config.LogPrefix, ck.BaseClerk.Cid, xid)
	log.Printf("%sstart new query request, num %d", logHeader, num)
	payload := SCPayLoad{Num: num}
	replyValue := ck.BaseClerk.DoRequest(&payload, OP_QUERY, xid, 1)
	return replyValue.(SCReplyValue).Config
}

func (ck *Clerk) Join(servers map[int][]string) {
	xid := ck.BaseClerk.XidGenerator.NextUid()
	logHeader := fmt.Sprintf("%sCk %d: xid %d: ",
		ck.BaseClerk.Config.LogPrefix, ck.BaseClerk.Cid, xid)
	log.Printf("%sstart new join request, servers %v", logHeader, servers)
	payload := SCPayLoad{Servers: servers}
	ck.BaseClerk.DoRequest(&payload, OP_JOIN, xid, 1)
}

func (ck *Clerk) Leave(gids []int) {
	xid := ck.BaseClerk.XidGenerator.NextUid()
	logHeader := fmt.Sprintf("%sCk %d: xid %d: ",
		ck.BaseClerk.Config.LogPrefix, ck.BaseClerk.Cid, xid)
	log.Printf("%sstart new leave request, gids %v", logHeader, gids)
	payload := SCPayLoad{GIDs: gids}
	ck.BaseClerk.DoRequest(&payload, OP_LEAVE, xid, 1)
}

func (ck *Clerk) Move(shard int, gid int) {
	xid := ck.BaseClerk.XidGenerator.NextUid()
	logHeader := fmt.Sprintf("%sCk %d: xid %d: ",
		ck.BaseClerk.Config.LogPrefix, ck.BaseClerk.Cid, xid)
	log.Printf("%sstart new move request, shard %d, gid %d", logHeader, shard, gid)
	payload := SCPayLoad{Shard: shard, GID: gid}
	ck.BaseClerk.DoRequest(&payload, OP_MOVE, xid, 1)
}
