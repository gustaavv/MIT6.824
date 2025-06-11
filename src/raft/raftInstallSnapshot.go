package raft

import (
	"fmt"
	"log"
)

type SnapShot struct {
	Data              []byte
	LastIncludedIndex int
	LastIncludedTerm  int
}

// Snapshot
// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (2D).

	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()

	if !rf.log.checkValidIndex(index) {
		log.Printf("inst %d: snapshot: index out of range: %d not in [%d, %d]",
			rf.me, index, rf.log.first().Index, rf.log.last().Index)
		return
	}
	if rf.SnapShot.LastIncludedIndex >= index {
		log.Printf("inst %d: snapshot: old snapshot index %d >= new snapshot index %d",
			rf.me, rf.SnapShot.LastIncludedIndex, index)
		return
	}
	if rf.lastApplied < index {
		log.Printf("inst %d: snapshot: lastApplied %d < new snapshot index %d",
			rf.me, rf.lastApplied, index)
		return
	}

	newSnapshot := SnapShot{Data: snapshot, LastIncludedIndex: index, LastIncludedTerm: rf.log.get(index).Term}
	newLogEntries := rf.log.getRangeStartFrom(index + 1)
	// make a new slice so that the original one can be GCed
	newLogEntriesCopy := make([]LogEntry, len(newLogEntries))
	copy(newLogEntriesCopy, newLogEntries)
	rf.log.Entries = newLogEntriesCopy
	rf.SnapShot = newSnapshot
	log.Printf("inst %d: snapshot: new snapshot created: LastIndex: %d, LastTerm: %d",
		rf.me, index, newSnapshot.LastIncludedTerm)
}

type InstallSnapshotArgs struct {
	TraceId           int
	Term              int
	LeaderId          int
	LastIncludedIndex int
	LastIncludedTerm  int
	Data              []byte
}

type InstallSnapshotReply struct {
	TraceId int
	Term    int
}

func (rf *Raft) sendISRequestAndHandleReply(peerIndex int) {
	peer := rf.peers[peerIndex]

	rf.mu.Lock()
	if rf.state != STATE_LEADER {
		rf.mu.Unlock()
		return
	}
	traceId := getNextTraceId()
	term := rf.currentTerm
	leaderId := rf.me
	lastIncludedIndex := rf.SnapShot.LastIncludedIndex
	lastIncludedTerm := rf.SnapShot.LastIncludedTerm
	data := make([]byte, len(rf.SnapShot.Data))
	copy(data, rf.SnapShot.Data)
	rf.mu.Unlock()

	args := new(InstallSnapshotArgs)
	args.TraceId = traceId
	args.Term = term
	args.LeaderId = leaderId
	args.LastIncludedIndex = lastIncludedIndex
	args.LastIncludedTerm = lastIncludedTerm
	args.Data = data

	reply := new(InstallSnapshotReply)
	ok := peer.Call("Raft.InstallSnapshot", args, reply)

	if !ok { // network failure
		return
	}

	// verify reply
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.state != STATE_LEADER { // only leader needs to handle InstallSnapshot RPC reply
		return
	}

	logHeader := fmt.Sprintf("inst %d: IS Resp: Trace: %d: ", rf.me, reply.TraceId)

	if reply.Term > rf.currentTerm {
		log.Printf("%sleader becomes follower because new leader %d with higher term: %d -> %d",
			logHeader, peerIndex, rf.currentTerm, reply.Term)
		rf.currentTerm = reply.Term
		rf.votedFor = -1
		rf.electionTimeoutAt = getNextElectionTimeout()
		rf.state = STATE_FOLLOWER
		rf.persist()
		return
	}

	rf.nextIndex[peerIndex] = max(lastIncludedIndex+1, rf.nextIndex[peerIndex])
	rf.successiveLogConflict[peerIndex] = SUCCESSIVE_CONFLICT_OFFSET
	log.Printf("%sinst %d's new nextIndex: %d", logHeader, peerIndex, rf.nextIndex[peerIndex])
}

func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	reply.TraceId = args.TraceId
	rf.mu.Lock()

	logHeader := fmt.Sprintf("inst %d: IS Req: Trace: %d: ", rf.me, args.TraceId)

	// #1
	if args.Term < rf.currentTerm { // outdated leader
		reply.Term = rf.currentTerm
		rf.mu.Unlock()
		return
	}

	rf.electionTimeoutAt = getNextElectionTimeout()
	reply.Term = args.Term

	if args.Term > rf.currentTerm {
		log.Printf("%s%v becomes follower because leader (inst %d) with higher term: %d -> %d",
			logHeader, rf.state, args.LeaderId, rf.currentTerm, args.Term)
		rf.currentTerm = args.Term
		rf.votedFor = -1
		rf.state = STATE_FOLLOWER
		rf.persist()
	}

	// #2 - #5 passed
	// #6 - #8 is in CondInstallSnapshot
	log.Printf("%sreceive snapshot from inst %d, lastLogIndex: %d, lastLogTerm: %d. lastApplied: %d, commitIndex: %d",
		logHeader, args.LeaderId, args.LastIncludedIndex, args.LastIncludedTerm, rf.lastApplied, rf.commitIndex)
	rf.mu.Unlock()

	// sending to channel should not be in critical sections
	rf.applyCh <- ApplyMsg{
		CommandValid:  false,
		SnapshotValid: true,
		Snapshot:      args.Data,
		SnapshotIndex: args.LastIncludedIndex,
		SnapshotTerm:  args.LastIncludedTerm,
	}
}

// CondInstallSnapshot
// A service wants to switch to snapshot.  Only do so if Raft hasn't
// have more recent info since it communicate the snapshot on applyCh.
//
func (rf *Raft) CondInstallSnapshot(lastIncludedTerm int, lastIncludedIndex int, snapshot []byte) bool {
	// Your code here (2D).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	// #6 there exists a log entry matching lastIncludedTerm and lastIncludedIndex
	if lastEntry := rf.log.get(lastIncludedIndex); lastEntry != nil && lastEntry.Term == lastIncludedTerm {
		if rf.lastApplied <= lastIncludedIndex {
			log.Printf("inst %d: CondIS: keep state: lastApplied %d <= lastIncludedIndex %d",
				rf.me, rf.lastApplied, lastIncludedIndex)
			// do nothing, since lastApplied will eventually catch up lastIncludedIndex
			return false
		} else if rf.SnapShot.LastIncludedIndex < lastIncludedIndex { // only keep the latest snapshot
			log.Printf("inst %d: CondIS: keep state: old lastIncludedIndex %d <= new lastIncludedIndex %d",
				rf.me, rf.SnapShot.LastIncludedIndex, lastIncludedIndex)
			// trim logs entries before LastIncludedIndex(including)
			// i.e., retain log entries after LastIncludedIndex
			newLogEntries := rf.log.getRangeStartFrom(lastIncludedIndex + 1)
			rf.log.Entries = make([]LogEntry, len(newLogEntries))
			copy(rf.log.Entries, newLogEntries)
			// save the snapshot
			rf.SnapShot.LastIncludedIndex = lastIncludedIndex
			rf.SnapShot.LastIncludedTerm = lastIncludedTerm
			rf.SnapShot.Data = snapshot

			rf.persist()

			return false
		}

	} else if rf.lastApplied < lastIncludedIndex {
		log.Printf("inst %d: CondIS: Reset state: lastApplied %d <= lastIncludedIndex %d",
			rf.me, rf.lastApplied, lastIncludedIndex)

		// #7 discard the entire log
		rf.log.Entries = make([]LogEntry, 0)
		// save the snapshot
		rf.SnapShot.LastIncludedIndex = lastIncludedIndex
		rf.SnapShot.LastIncludedTerm = lastIncludedTerm
		rf.SnapShot.Data = snapshot
		// update other states
		rf.commitIndex = lastIncludedIndex
		rf.lastApplied = lastIncludedIndex

		rf.persist()

		// #8 reset state machine
		return true
	}

	log.Printf("inst %d: CondIS: old useless snapshot: lastIncludedTerm: %d, lastIncludedIndex: %d, lastApplied: %d",
		rf.me, lastIncludedTerm, lastIncludedIndex, rf.lastApplied)

	return false
}
