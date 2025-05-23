package mr

//
// RPC definitions.
//
// remember to capitalize all names.
//

import (
	"fmt"
	"os"
	"time"
)
import "strconv"

// Add your RPC definitions here.

type WorkerPingArgs struct {
	WorkerId int
	TraceId  string
	State    string
	// for successfully finished map/reduce tasks
	MapNumber    int
	ReduceNumber int
}

func (args *WorkerPingArgs) String() string {
	return fmt.Sprintf("{worker id=%d trace id=%s state=%s mapNumber=%d reduceNumber=%d}",
		args.WorkerId, args.TraceId, args.State, args.MapNumber, args.ReduceNumber)
}

// Reply from coordinator
type WorkerPingReply struct {
	TraceId string
	Order   string
	R       int // reduce partition constant
	// for assigning map/reduce tasks to workers
	MapNumber    int
	MapFile      string
	ReduceNumber int
}

func (reply *WorkerPingReply) String() string {
	return fmt.Sprintf("{trace id=%s order=%s R=%d mapNumber=%d mapFile=%s reduceNumber=%d}",
		reply.TraceId, reply.Order, reply.R, reply.MapNumber, reply.MapFile, reply.ReduceNumber)
}

func generateTraceId() string {
	return strconv.FormatInt(time.Now().UnixNano(), 10) + strconv.Itoa(os.Getpid())
}

// Cook up a unique-ish UNIX-domain socket name
// in /var/tmp, for the coordinator.
// Can't use the current directory since
// Athena AFS doesn't support UNIX-domain sockets.
func coordinatorSock() string {
	s := "/var/tmp/824-mr-"
	s += strconv.Itoa(os.Getuid())
	return s
}
