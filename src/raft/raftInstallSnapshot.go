package raft

import "log"

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
	if rf.log.SnapShot.LastIncludedIndex >= index {
		log.Printf("inst %d: snapshot: old snapshot index %d >= new snapshot index %d",
			rf.me, rf.log.SnapShot.LastIncludedIndex, index)
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
	rf.log.SnapShot = newSnapshot
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
	term := rf.currentTerm
	leaderId := rf.me
	lastIncludedIndex := rf.log.SnapShot.LastIncludedIndex
	lastIncludedTerm := rf.log.SnapShot.LastIncludedTerm
	data := make([]byte, len(rf.log.SnapShot.Data))
	copy(data, rf.log.SnapShot.Data)
	rf.mu.Unlock()

	args := new(InstallSnapshotArgs)
	args.TraceId = getNextTraceId()
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

	if reply.Term > rf.currentTerm {
		log.Printf("inst %d: IS Resp: leader becomes follower because new leader %d with higher term: %d -> %d",
			rf.me, peerIndex, rf.currentTerm, reply.Term)
		rf.currentTerm = reply.Term
		rf.votedFor = -1
		rf.electionTimeoutAt = getNextElectionTimeout()
		rf.state = STATE_FOLLOWER
		rf.persist()
		return
	}

	rf.nextIndex[peerIndex] = max(lastIncludedIndex+1, rf.nextIndex[peerIndex])
	rf.successiveLogConflict[peerIndex] = 0
	log.Printf("inst %d: IS Resp: inst %d's new nextIndex: %d", rf.me, peerIndex, rf.nextIndex[peerIndex])
}

func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	reply.TraceId = args.TraceId
	rf.mu.Lock()

	// #1
	if args.Term < rf.currentTerm { // outdated leader
		reply.Term = rf.currentTerm
		rf.mu.Unlock()
		return
	}

	rf.electionTimeoutAt = getNextElectionTimeout()
	reply.Term = args.Term

	if args.Term > rf.currentTerm {
		log.Printf("inst %d: AE Req: %v becomes follower because leader (inst %d) with higher term: %d -> %d",
			rf.me, rf.state, args.LeaderId, rf.currentTerm, args.Term)
		rf.currentTerm = args.Term
		rf.votedFor = -1
		rf.state = STATE_FOLLOWER
		rf.persist()
	}

	// #2 - #5 passed
	// #6 - #8 is in CondInstallSnapshot
	log.Printf("inst %d: IS Req: receive snapshot from inst %d, lastLogIndex: %d, lastLogTerm: %d. lastApplied: %d, commitIndex: %d",
		rf.me, args.LeaderId, args.LastIncludedIndex, args.LastIncludedTerm, rf.lastApplied, rf.commitIndex)
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
	defer rf.persist()

	// #6 there exists a log entry matching lastIncludedTerm and lastIncludedIndex
	if lastEntry := rf.log.get(lastIncludedIndex); lastEntry != nil && lastEntry.Term == lastIncludedTerm {
		if rf.lastApplied <= lastIncludedIndex {
			log.Printf("inst %d: CondIS: keep state: lastApplied %d <= lastIncludedIndex %d",
				rf.me, rf.lastApplied, lastIncludedIndex)
			// TODO: do nothing? Since lastApplied will eventually catch up lastIncludedIndex
			return false
		} else if rf.log.SnapShot.LastIncludedIndex < lastIncludedIndex { // only keep the latest snapshot
			log.Printf("inst %d: CondIS: keep state: old lastIncludedIndex %d <= new lastIncludedIndex %d",
				rf.me, rf.log.SnapShot.LastIncludedIndex, lastIncludedIndex)
			// trim logs entries before LastIncludedIndex(including)
			// i.e., retain log entries after LastIncludedIndex
			newLogEntries := rf.log.getRangeStartFrom(lastIncludedIndex + 1)
			rf.log.Entries = make([]LogEntry, len(newLogEntries))
			copy(rf.log.Entries, newLogEntries)
			// save the snapshot
			rf.log.SnapShot.LastIncludedIndex = lastIncludedIndex
			rf.log.SnapShot.LastIncludedTerm = lastIncludedTerm
			rf.log.SnapShot.Data = snapshot

			return false
		}

	} else if rf.lastApplied < lastIncludedIndex {
		log.Printf("inst %d: CondIS: Reset state: lastApplied %d <= lastIncludedIndex %d",
			rf.me, rf.lastApplied, lastIncludedIndex)

		// #7 discard the entire log
		rf.log.Entries = make([]LogEntry, 0)
		// save the snapshot
		rf.log.SnapShot.LastIncludedIndex = lastIncludedIndex
		rf.log.SnapShot.LastIncludedTerm = lastIncludedTerm
		rf.log.SnapShot.Data = snapshot
		// TODO: update other states?
		rf.commitIndex = lastIncludedIndex
		rf.lastApplied = lastIncludedIndex

		// #8 reset state machine
		return true
	}

	return false
}
