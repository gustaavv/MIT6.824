package atopraft

import "fmt"

const (
	MSG_OLD_XID        = "OLD_XID"
	MSG_NOT_LEADER     = "NOT_LEADER"
	MSG_MULTIPLE_XID   = "MULTIPLE_XID"
	MSG_OP_UNSUPPORTED = "OP_UNSUPPORTED"
	MSG_SHUTDOWN       = "SHUTDOWN"
	MSG_READ_SNAPSHOT  = "READ_SNAPSHOT"
	MSG_UNAVAILABLE    = "UNAVAILABLE"
)

type SrvArgs struct {
	Cid int
	Xid int
	Tid int
	Op  string
	// This field is either ArgsPayLoad or nil
	PayLoad interface{}
}

func (sa *SrvArgs) String() string {
	ps := "nil"
	if sa.PayLoad != nil {
		ps = sa.PayLoad.(ArgsPayLoad).String()
	}
	return fmt.Sprintf("SrvArgs{Cid: %d, Xid: %d, Tid: %d, Op: %q, PayLoad: %s}",
		sa.Cid, sa.Xid, sa.Tid, sa.Op, ps)
}

// ArgsPayLoad The implementer must use value type rather than pointer type
type ArgsPayLoad interface {
	String() string
	Clone() ArgsPayLoad
}

type SrvReply struct {
	Success bool
	Msg     string
	// This field is either ReplyValue or nil
	Value interface{}
}

func (sr *SrvReply) String() string {
	vs := "nil"
	if sr.Value != nil {
		vs = sr.Value.(ReplyValue).String()
	}
	return fmt.Sprintf("SrvReply{Success: %v, Msg: %q, Value: %s}",
		sr.Success, sr.Msg, vs)
}

// ReplyValue The implementer must use value type rather than pointer type
type ReplyValue interface {
	String() string
	Clone() ReplyValue
}
