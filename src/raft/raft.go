package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"bytes"
	"log"
	"time"

	//	"bytes"
	"sync"
	"sync/atomic"

	"6.824/labgob"
	//	"6.824/labgob"
	"6.824/labrpc"
)

// ApplyMsg
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part 2D you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For 2D:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

// Raft
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()
	applyCh   chan ApplyMsg

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.

	// states on Figure 2 ///////////////////////////////////

	// persistent on all servers
	currentTerm int
	votedFor    int // use -1 as null
	log         raftLog

	// volatile on all servers
	commitIndex int
	lastApplied int

	// volatile on leaders
	nextIndex  []int
	matchIndex []int

	// states not on Figure 2 ////////////////////////////////

	// volatile

	state                    string
	electionTimeoutAt        time.Time // for election timeout cronjob
	heartbeatAt              time.Time // for heartbeat cronjob
	lastNewEntryIndex        int       // see Figure 2 | AE RPC | Receiver #5, index of last new entry
	applyLogEntryAt          time.Time // for apply log entry cronjob
	firstLogIndexCurrentTerm int       // see Figure 2 | Rules for servers | Leaders last rule
	successiveLogConflict    []int     // for LOG_BT_BIN_EXP
}

func (rf *Raft) initVolatileLeaderState() {
	// initialized to leader last log index + 1
	for i := range rf.nextIndex {
		rf.nextIndex[i] = rf.log.nextIndex()
	}
	for i := range rf.matchIndex {
		rf.matchIndex[i] = 0
	}

	// states not on Figure 2 ////////////////////////////////
	// for convenience, put the init/re-init of relevant fields here

	for i := range rf.successiveLogConflict {
		rf.successiveLogConflict[i] = 0
	}
}

// GetState
// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	// Your code here (2A).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	term := rf.currentTerm
	isLeader := rf.state == STATE_LEADER
	//log.Printf("inst %v: term:%v state:%v", rf.me, rf.currentTerm, rf.state)
	return term, isLeader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// This function must be called in a critical section
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)

	//log.Printf("inst %d: persist log start", rf.me)

	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	if err := e.Encode(rf.currentTerm); err != nil {
		log.Fatalf("inst %d: persist log err: %v", rf.me, err)
	}
	if err := e.Encode(rf.votedFor); err != nil {
		log.Fatalf("inst %d: persist log err: %v", rf.me, err)
	}
	if err := e.Encode(rf.log); err != nil {
		log.Fatalf("inst %d: persist log err: %v", rf.me, err)
	}

	data := w.Bytes()

	// TODO: Hint #6: use SaveStateAndSnapshot()
	rf.persister.SaveRaftState(data)
	//log.Printf("inst %d: persist log finished, data len: %d", rf.me, len(data))
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }

	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)

	var currentTerm int
	var votedFor int
	var raftLog raftLog

	if d.Decode(&currentTerm) != nil ||
		d.Decode(&votedFor) != nil ||
		d.Decode(&raftLog) != nil {
		log.Fatalf("inst %d: readPersist: error happens when decoding", rf.me)
	} else {
		rf.currentTerm = currentTerm
		rf.votedFor = votedFor
		rf.log = raftLog
	}
	log.Printf("inst %d: readPersist: restore previously persisted state, currentTerm: %d, votedFor: %d",
		rf.me, rf.currentTerm, votedFor)
}

// Start
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	// Your code here (2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	index := rf.log.SnapShot.LastIncludedIndex + 1
	if !rf.log.isEmpty() {
		index = rf.log.nextIndex()
	}
	term := rf.currentTerm
	isLeader := rf.state == STATE_LEADER

	if !isLeader {
		return -7, -23, isLeader
	}

	rf.log.append(LogEntry{Term: term, Command: command, Index: index})
	rf.persist()
	log.Printf("inst %d: Start: leader appends a new log entry (index %d)", rf.me, index)

	// TODO: send AE RPC immediately. Create a new goroutine to send.

	return index, term, isLeader
}

// Kill
// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
//
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

// Make
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.applyCh = applyCh
	// Your initialization code here (2A, 2B, 2C).
	configLog()
	validateLogBacktrackingMode()

	// states on Figure 2 ///////////////////////////////////

	// persistent on all servers

	rf.currentTerm = 0
	rf.votedFor = -1
	rf.log.Entries = make([]LogEntry, 0)
	// log index begins at 1 and instances always need to agree on the very first log entry
	rf.log.append(LogEntry{Term: -1, Index: 0})

	// volatile on all servers

	rf.commitIndex = 0
	rf.lastApplied = 0

	// volatile on leaders
	rf.nextIndex = make([]int, len(rf.peers))
	rf.matchIndex = make([]int, len(rf.peers))

	// states not on Figure 2 ////////////////////////////////

	rf.state = STATE_FOLLOWER
	rf.electionTimeoutAt = getNextElectionTimeout()
	rf.heartbeatAt = time.Now()
	rf.lastNewEntryIndex = -1
	rf.applyLogEntryAt = getNextApplyLogEntryTime()
	rf.firstLogIndexCurrentTerm = 0
	rf.successiveLogConflict = make([]int, len(rf.peers))

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	// nextIndex is based on log[], so the init should be after reading persistent state
	rf.initVolatileLeaderState()
	// lastApplied points to the first index - 1 on init, and it needs to be greater than 0
	rf.lastApplied = max(rf.log.first().Index-1, 0)

	log.Printf("inst %d: start as follower", rf.me)

	// start electionTimeoutTicker goroutine to start elections
	go rf.electionTimeoutTicker()
	go rf.heartbeatTicker()
	go rf.applyLogEntryTicker()

	return rf
}
