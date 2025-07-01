package kvraft

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
	cid := clerkIdGenerator.NextUid()
	ck.BaseClerk = atopraft.MakeBaseClerk(cid, servers, makeKVConfig(), "KVServer")

	return ck
}

// Get
// fetch the current value for a key.
// returns "" if the key does not exist.
// keeps trying forever in the face of all other errors.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.Get", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
func (ck *Clerk) Get(key string) string {
	// You will have to modify this function.
	xid := ck.BaseClerk.XidGenerator.NextUid()
	logHeader := fmt.Sprintf("%sCk %d: xid %d: ",
		ck.BaseClerk.Config.LogPrefix, ck.BaseClerk.Cid, xid)
	log.Printf("%sstart new Get request, key %q", logHeader, key)
	payload := KVPayLoad{
		Key:   key,
		Value: "",
	}
	replyValue := ck.BaseClerk.DoRequest(&payload, OP_GET, xid, 1)
	return string(replyValue.(KVReplyValue))
}

// PutAppend
// shared by Put and Append.
//
// you can send an RPC with code like this:
// ok := ck.servers[i].Call("KVServer.PutAppend", &args, &reply)
//
// the types of args and reply (including whether they are pointers)
// must match the declared types of the RPC handler function's
// arguments. and reply must be passed as a pointer.
//
func (ck *Clerk) PutAppend(key string, value string, op string) {
	if !(op == OP_PUT || op == OP_APPEND) {
		log.Fatal("op not support", op)
	}

	// You will have to modify this function.
	xid := ck.BaseClerk.XidGenerator.NextUid()
	logHeader := fmt.Sprintf("%sCk %d: xid %d: ",
		ck.BaseClerk.Config.LogPrefix, ck.BaseClerk.Cid, xid)
	value2 := KVReplyValue(value)
	log.Printf("%sstart new %s request, key %q value %q",
		logHeader, op, key, atopraft.LogV(&value2, ck.BaseClerk.Config))
	payload := KVPayLoad{
		Key:   key,
		Value: value,
	}
	ck.BaseClerk.DoRequest(&payload, op, xid, 1)
}

func (ck *Clerk) Put(key string, value string) {
	ck.PutAppend(key, value, OP_PUT)
}
func (ck *Clerk) Append(key string, value string) {
	ck.PutAppend(key, value, OP_APPEND)
}
