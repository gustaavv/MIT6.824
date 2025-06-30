package kvraft

import "fmt"

const (
	OP_GET    = "Get"
	OP_PUT    = "Put"
	OP_APPEND = "Append"
)

const (
	MSG_OLD_XID        = "OLD_XID"
	MSG_NOT_LEADER     = "NOT_LEADER"
	MSG_MULTIPLE_XID   = "MULTIPLE_XID"
	MSG_OP_UNSUPPORTED = "OP_UNSUPPORTED"
	MSG_SHUTDOWN       = "SHUTDOWN"
	MSG_READ_SNAPSHOT  = "READ_SNAPSHOT"
	MSG_UNAVAILABLE    = "UNAVAILABLE"
)

type KVArgs struct {
	Cid int
	Xid int
	Tid int

	Key   string
	Value string
	Op    string // Get, Put, Append
}

func (kva *KVArgs) String() string {
	return fmt.Sprintf(
		"KVArgs{Cid: %d, Xid: %d, Tid: %d, Key: %q, Value: %q, Op: %q}",
		kva.Cid, kva.Xid, kva.Tid, kva.Key, logV(kva.Value), kva.Op,
	)
}

type KVReply struct {
	Success bool
	Msg     string
	Value   string
}

func (kvr *KVReply) String() string {
	return fmt.Sprintf(
		"KVReply{Success: %v, Msg: %q, Value: %q}",
		kvr.Success, kvr.Msg, logV(kvr.Value),
	)
}
