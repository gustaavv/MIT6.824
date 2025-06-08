package raft

import (
	"log"
	"time"
)

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool

	// for LOG_BT_TERM_BYPASS

	ConflictIndex int // use -1 as null
	ConflictTerm  int // use -1 as null
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()

	// #1
	if args.Term < rf.currentTerm { // outdated leader
		reply.Term = rf.currentTerm
		reply.Success = false
		return
	}

	// it's safe to reset election timeout for both follower and candidate state here
	// because there is only one leader per term
	rf.electionTimeoutAt = getNextElectionTimeout()
	reply.Term = args.Term

	if args.Term > rf.currentTerm {
		log.Printf("inst %d: AE Req: %v becomes follower because leader (inst %d) with higher term: %d -> %d",
			rf.me, rf.state, args.LeaderId, rf.currentTerm, args.Term)
		rf.currentTerm = args.Term
		rf.votedFor = -1
		rf.state = STATE_FOLLOWER
	}

	if LOG_BACKTRACKING_MODE == LOG_BT_TERM_BYPASS && !rf.log.isEmpty() {
		if args.PrevLogIndex >= rf.log.nextIndex() {
			reply.ConflictTerm = -1
			reply.ConflictIndex = rf.log.nextIndex()
		} else if entry := rf.log.get(args.PrevLogIndex); entry != nil && entry.Term != args.PrevLogTerm {
			reply.ConflictTerm = entry.Term

			// the student's guide does not say to store the search result into ConflictIndex.
			// But I guess we should do this
			reply.ConflictIndex = args.PrevLogIndex
			for reply.ConflictIndex-1 >= rf.log.first().Index &&
				rf.log.get(reply.ConflictIndex-1).Term == reply.ConflictTerm {
				reply.ConflictIndex--
			}
		}
	}

	// #2
	if entry := rf.log.get(args.PrevLogIndex); !rf.log.isEmpty() && (entry == nil || entry.Term != args.PrevLogTerm) {
		reply.Success = false
		return
	}
	if rf.log.isEmpty() &&
		!(rf.log.SnapShot.LastIncludedIndex == args.PrevLogIndex &&
			rf.log.SnapShot.LastIncludedTerm == args.PrevLogTerm) {
		reply.Success = false
		return
	}

	reply.Success = true

	// #3
	i := args.PrevLogIndex + 1
	j := 0
	for !rf.log.isEmpty() && i < rf.log.nextIndex() && j < len(args.Entries) {
		if rf.log.get(i).Term != args.Entries[j].Term { // conflict happens.
			rf.log.Entries = rf.log.getRange(rf.log.first().Index, i-1) // delete existing entries
			//i = rf.log.nextIndex()
			break
		}
		i++
		j++
	}

	// #4
	if j < len(args.Entries) { // new entries
		// data race happens when appending args.Entries[j:] directly into rf.log
		// I don't know why gob read cause such data race
		entriesToAppend := make([]LogEntry, len(args.Entries[j:]))
		copy(entriesToAppend, args.Entries[j:])

		rf.log.append(entriesToAppend...)
		rf.lastNewEntryIndex = rf.log.last().Index
	}

	// #5
	if args.LeaderCommit > rf.commitIndex && rf.lastNewEntryIndex != -1 {
		rf.commitIndex = min(args.LeaderCommit, rf.lastNewEntryIndex)
		log.Printf("inst %d: AE Req: follower commits new logs (commitIndex: %d)", rf.me, rf.commitIndex)
	}
}

func getNextHeartbeatTime() time.Time {
	return time.Now().Add(HEARTBEAT_FERQUENCY)
}

func (rf *Raft) sendAERequestAndHandleReply(peerIndex int) {
	peer := rf.peers[peerIndex]

	// copy rf's state before putting into args
	rf.mu.Lock()
	currentTerm := rf.currentTerm
	leaderId := rf.me

	if nextIndex := rf.nextIndex[peerIndex]; nextIndex <= rf.log.SnapShot.LastIncludedIndex {
		// a lagging follower
		log.Printf("inst %d: AE Req => IS Req: inst %d's nextIndex %d <= snapshot.LastIncludedIndex %d",
			rf.me, peerIndex, nextIndex, rf.log.SnapShot.LastIncludedIndex)
		rf.mu.Unlock()
		go rf.sendISRequestAndHandleReply(peerIndex)
		return
	}

	prevLogIndex := rf.log.SnapShot.LastIncludedIndex
	prevLogTerm := rf.log.SnapShot.LastIncludedTerm

	if pli := rf.nextIndex[peerIndex] - 1; rf.log.get(pli) != nil {
		prevLogIndex = pli
		prevLogTerm = rf.log.get(prevLogIndex).Term
	}

	entries := make([]LogEntry, 0)
	if !rf.log.isEmpty() {
		entries = make([]LogEntry, rf.log.nextIndex()-rf.nextIndex[peerIndex])
		copy(entries, rf.log.getRangeStartFrom(rf.nextIndex[peerIndex]))
	}

	leaderCommit := rf.commitIndex
	rf.mu.Unlock()

	args := new(AppendEntriesArgs)
	args.Term = currentTerm
	args.LeaderId = leaderId
	args.PrevLogIndex = prevLogIndex
	args.PrevLogTerm = prevLogTerm
	args.Entries = entries
	args.LeaderCommit = leaderCommit

	reply := new(AppendEntriesReply)
	ok := peer.Call("Raft.AppendEntries", args, reply)

	if !ok { // network failure
		return
	}

	// verify reply
	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.state != STATE_LEADER { // only leader needs to handle AppendEntries RPC reply
		return
	}

	if reply.Term > rf.currentTerm {
		log.Printf("inst %d: AE Resp: leader becomes follower because new leader %d with higher term: %d -> %d",
			rf.me, peerIndex, rf.currentTerm, reply.Term)
		rf.currentTerm = reply.Term
		rf.votedFor = -1
		rf.electionTimeoutAt = getNextElectionTimeout()
		rf.state = STATE_FOLLOWER
		rf.persist()
	}

	if args.Term != reply.Term {
		return
	}

	if reply.Success {
		// we need to guard against old reply in the same term, which acknowledged old (smaller) matchIndex
		// because matchIndex should not go back
		if prevLogIndex+len(entries) > rf.matchIndex[peerIndex] {
			rf.matchIndex[peerIndex] = prevLogIndex + len(entries)
			if LOG_BACKTRACKING_MODE != LOG_BT_AGGRESSIVE {
				rf.nextIndex[peerIndex] = rf.matchIndex[peerIndex] + 1
			}
		}
		rf.successiveLogConflict[peerIndex] = 0
	} else {
		if LOG_BACKTRACKING_MODE == LOG_BT_TERM_BYPASS {
			i := rf.log.last().Index
			for ; i > rf.log.first().Index; i-- {
				if rf.log.get(i).Term == reply.ConflictTerm {
					break
				}
			}

			if i > 0 {
				rf.nextIndex[peerIndex] = i + 1
			} else {
				rf.nextIndex[peerIndex] = reply.ConflictIndex
			}
		}

		if LOG_BACKTRACKING_MODE == LOG_BT_ORIGINAL {
			rf.nextIndex[peerIndex]--
		}

		if LOG_BACKTRACKING_MODE == LOG_BT_AGGRESSIVE {
			rf.nextIndex[peerIndex] = 1
		}

		if LOG_BACKTRACKING_MODE == LOG_BT_BIN_EXP {
			rf.nextIndex[peerIndex] -= 1 << rf.successiveLogConflict[peerIndex]
			rf.nextIndex[peerIndex] = max(rf.nextIndex[peerIndex], 1)
		}

		rf.successiveLogConflict[peerIndex]++
	}

	// rf.firstLogIndexCurrentTerm ensures that log[N].term == currentTem
	// try to find an N s.t. N > commitIndex, so just let N = commitIndex + 1
	N := max(rf.commitIndex+1, rf.firstLogIndexCurrentTerm)
	majorityCount := len(rf.peers) / 2
	for i, mi := range rf.matchIndex {
		if i == rf.me {
			continue
		}
		if mi >= N {
			majorityCount--
		}
	}
	if majorityCount <= 0 {
		rf.commitIndex = N
		if rf.log.get(N).Term != currentTerm { // double check whether my implementation is correct
			log.Fatalf("inst %d: AE Resp: Error: the newly committed log's term is %d, but current term is %d",
				rf.me, rf.log.get(N).Term, currentTerm)
		}
		log.Printf("inst %d: AE Resp: leader commits new logs (commitIndex: %d)", rf.me, rf.commitIndex)
	}
}

// This function must be called in the critical section
func (rf *Raft) sendHeartbeats() {
	for i := range rf.peers {
		if i == rf.me {
			continue
		}
		go rf.sendAERequestAndHandleReply(i)
	}
}

func (rf *Raft) heartbeatTicker() {
	for rf.killed() == false {
		time.Sleep(TICKER_FREQUENCY)

		rf.mu.Lock()

		if rf.state != STATE_LEADER { // only leader sends heartbeat
			rf.mu.Unlock()
			continue
		}

		if rf.heartbeatAt.After(time.Now()) { // no need to send heartbeat now
			rf.mu.Unlock()
			continue
		}

		//log.Printf("inst %d: ticker: heartbeat at %v", rf.me, time.Now())

		rf.sendHeartbeats()

		rf.heartbeatAt = getNextHeartbeatTime()

		rf.mu.Unlock()
	}
}
