package raft

import (
	"fmt"
	"log"
)

type SnapShot struct {
	Data              []byte
	LastIncludedIndex int
	LastIncludedTerm  int
	Id                int
}

// Snapshot
// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) bool {
	// Your code here (2D).

	rf.mu.Lock()
	defer rf.mu.Unlock()

	if !rf.enableSnapshot {
		return false
	}

	logHeader := fmt.Sprintf("inst %d: snapshot: ", rf.me)

	if !rf.log.isEmpty() && index < rf.log.first().Index-1 {
		if ENABLE_RAFT_LOG {
			log.Printf("%ssnapshot lastIncludedIndex %d, log index range [%d, %d]",
				logHeader, index, rf.log.first().Index, rf.log.last().Index)
		}
		return false
	}
	if rf.SnapShot.LastIncludedIndex >= index {
		if ENABLE_RAFT_LOG {
			log.Printf("%sold snapshot index %d >= new snapshot index %d",
				logHeader, rf.SnapShot.LastIncludedIndex, index)
		}
		return false
	}
	if rf.lastApplied < index {
		if ENABLE_RAFT_LOG {
			log.Printf("%slastApplied %d < new snapshot index %d",
				logHeader, rf.lastApplied, index)
		}
		return false
	}

	// save the snapshot first
	newSnapshot := SnapShot{
		Data:              snapshot,
		LastIncludedIndex: index,
		LastIncludedTerm:  rf.log.get(index).Term,
		Id:                getNextSnapshotId(),
	}
	rf.SnapShot = newSnapshot
	rf.persist(true)

	// then save the state
	newLogEntries := rf.log.getRangeStartFrom(index + 1)
	// make a new slice so that the original one can be GCed
	newLogEntriesCopy := make([]LogEntry, len(newLogEntries))
	copy(newLogEntriesCopy, newLogEntries)
	rf.log.Entries = newLogEntriesCopy
	if ENABLE_RAFT_LOG {
		log.Printf("%snew snapshot created: LastIndex: %d, LastTerm: %d, Id: %d, bytes: %d",
			logHeader, index, newSnapshot.LastIncludedTerm, newSnapshot.Id, len(newSnapshot.Data))
	}
	rf.persist(false)

	rf.applyLogEntryMu.Lock()
	rf.applyLogEntryCond.Broadcast()
	rf.applyLogEntryMu.Unlock()

	return true
}

type InstallSnapshotArgs struct {
	TraceId           int
	Term              int
	LeaderId          int
	LastIncludedIndex int
	LastIncludedTerm  int
	Data              []byte
	SnapshotId        int
}

type InstallSnapshotReply struct {
	TraceId    int
	Term       int
	SnapshotId int
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
	snapshotId := rf.SnapShot.Id
	rf.mu.Unlock()

	args := new(InstallSnapshotArgs)
	args.TraceId = traceId
	args.Term = term
	args.LeaderId = leaderId
	args.LastIncludedIndex = lastIncludedIndex
	args.LastIncludedTerm = lastIncludedTerm
	args.Data = data
	args.SnapshotId = snapshotId

	reply := new(InstallSnapshotReply)
	ok := peer.Call("Raft.InstallSnapshot", args, reply)

	if !ok { // network failure
		return
	}

	// verify reply
	rf.mu.Lock()
	defer rf.mu.Unlock()

	logHeader := fmt.Sprintf("inst %d: IS Resp: Trace: %d: ", rf.me, reply.TraceId)

	if reply.Term > rf.currentTerm {
		if ENABLE_RAFT_LOG {
			log.Printf("%sleader becomes follower because new leader %d with higher term: %d -> %d",
				logHeader, peerIndex, rf.currentTerm, reply.Term)
		}
		rf.currentTerm = reply.Term
		rf.votedFor = -1
		rf.electionTimeoutAt = getNextElectionTimeout()
		rf.state = STATE_FOLLOWER
		rf.persist(false)
		return
	}

	if rf.state != STATE_LEADER { // only leader needs to handle InstallSnapshot RPC reply
		return
	}

	// do not forget to update nextIndex, otherwise there will always be AE Req => IS Req
	rf.nextIndex[peerIndex] = max(lastIncludedIndex+1, rf.nextIndex[peerIndex])
	rf.successiveLogConflict[peerIndex] = SUCCESSIVE_CONFLICT_OFFSET
	if ENABLE_RAFT_LOG {
		log.Printf("%sinst %d's new nextIndex: %d", logHeader, peerIndex, rf.nextIndex[peerIndex])
	}
}

func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	reply.TraceId = args.TraceId
	reply.SnapshotId = args.SnapshotId
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
		if ENABLE_RAFT_LOG {
			log.Printf("%s%v becomes follower because leader (inst %d) with higher term: %d -> %d",
				logHeader, rf.state, args.LeaderId, rf.currentTerm, args.Term)
		}
		rf.currentTerm = args.Term
		rf.votedFor = -1
		rf.state = STATE_FOLLOWER
		rf.persist(false)
	}

	// #2 - #5 passed
	// #6 - #8 is in CondInstallSnapshot
	if ENABLE_RAFT_LOG {
		log.Printf("%sreceive snapshot from inst %d, lastLogIndex: %d, lastLogTerm: %d, Id: %d. lastApplied: %d, commitIndex: %d",
			logHeader, args.LeaderId, args.LastIncludedIndex, args.LastIncludedTerm, args.SnapshotId, rf.lastApplied, rf.commitIndex)
	}
	rf.mu.Unlock()

	// sending to channel should not be in critical sections
	rf.applyCh <- ApplyMsg{
		CommandValid:  false,
		SnapshotValid: true,
		Snapshot:      args.Data,
		SnapshotIndex: args.LastIncludedIndex,
		SnapshotTerm:  args.LastIncludedTerm,
		SnapshotId:    args.SnapshotId,
	}
}

// CondInstallSnapshot
// A service wants to switch to snapshot.  Only do so if Raft hasn't
// have more recent info since it communicate the snapshot on applyCh.
//
func (rf *Raft) CondInstallSnapshot(lastIncludedTerm int, lastIncludedIndex int, snapshotId int, snapshot []byte) bool {
	// Your code here (2D).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if !rf.enableSnapshot {
		return false
	}

	logHeader := fmt.Sprintf("inst %d: CondIS: snapshotId: %d: ", rf.me, snapshotId)

	// #6 there exists a log entry matching lastIncludedTerm and lastIncludedIndex
	if lastEntry := rf.log.get(lastIncludedIndex); lastEntry != nil && lastEntry.Term == lastIncludedTerm {
		if rf.lastApplied <= lastIncludedIndex {
			if ENABLE_RAFT_LOG {
				log.Printf("%skeep state: lastApplied %d <= lastIncludedIndex %d",
					logHeader, rf.lastApplied, lastIncludedIndex)
			}
			// do nothing, since lastApplied will eventually catch up lastIncludedIndex
			return false
		} else if rf.SnapShot.LastIncludedIndex < lastIncludedIndex { // only keep the latest snapshot
			if ENABLE_RAFT_LOG {
				log.Printf("%skeep state: old lastIncludedIndex %d <= new lastIncludedIndex %d",
					logHeader, rf.SnapShot.LastIncludedIndex, lastIncludedIndex)
			}
			// save the snapshot first
			rf.SnapShot.LastIncludedIndex = lastIncludedIndex
			rf.SnapShot.LastIncludedTerm = lastIncludedTerm
			rf.SnapShot.Data = snapshot
			rf.persist(true)

			// then save the state
			// trim logs entries before LastIncludedIndex(including)
			// i.e., retain log entries after LastIncludedIndex
			newLogEntries := rf.log.getRangeStartFrom(lastIncludedIndex + 1)
			rf.log.Entries = make([]LogEntry, len(newLogEntries))
			copy(rf.log.Entries, newLogEntries)
			rf.persist(false)

			rf.applyLogEntryMu.Lock()
			rf.applyLogEntryCond.Broadcast()
			rf.applyLogEntryMu.Unlock()

			return false
		}

	} else if rf.lastApplied < lastIncludedIndex {
		if ENABLE_RAFT_LOG {
			log.Printf("%sReset state: lastApplied %d <= lastIncludedIndex %d",
				logHeader, rf.lastApplied, lastIncludedIndex)
		}
		// #7 discard the entire log
		rf.log.Entries = make([]LogEntry, 0)
		// save the snapshot
		rf.SnapShot.LastIncludedIndex = lastIncludedIndex
		rf.SnapShot.LastIncludedTerm = lastIncludedTerm
		rf.SnapShot.Data = snapshot
		// update other states
		rf.commitIndex = lastIncludedIndex
		rf.lastApplied = lastIncludedIndex

		rf.persist(true)

		rf.applyLogEntryMu.Lock()
		rf.applyLogEntryCond.Broadcast()
		rf.applyLogEntryMu.Unlock()

		// #8 reset state machine
		return true
	}
	if ENABLE_RAFT_LOG {
		log.Printf("%sold useless snapshot: lastIncludedTerm: %d, lastIncludedIndex: %d, lastApplied: %d",
			logHeader, lastIncludedTerm, lastIncludedIndex, rf.lastApplied)
	}
	return false
}
