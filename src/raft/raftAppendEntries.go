package raft

import (
	"log"
	"time"
)

type AppendEntriesArgs struct {
	TraceId      int
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	TraceId int
	Term    int
	Success bool

	// for LOG_BT_TERM_BYPASS

	ConflictIndex int // use -1 as null
	ConflictTerm  int // use -1 as null
}

func (rf *Raft) sendAERequestAndHandleReply(peerIndex int, conflictRetries int) {
	peer := rf.peers[peerIndex]

	// copy rf's state before putting into args
	rf.mu.Lock()
	currentTerm := rf.currentTerm
	leaderId := rf.me

	if nextIndex := rf.nextIndex[peerIndex]; nextIndex <= rf.SnapShot.LastIncludedIndex {
		// a lagging follower
		log.Printf("inst %d: AE Req => IS Req: inst %d's nextIndex %d <= snapshot.LastIncludedIndex %d",
			rf.me, peerIndex, nextIndex, rf.SnapShot.LastIncludedIndex)
		rf.mu.Unlock()
		go rf.sendISRequestAndHandleReply(peerIndex)
		return
	}

	if !rf.log.isEmpty() {
		log.Printf("inst %d: AE Req: inst %d's nextIndex: %d. log first entry index: %d, log next index: %d",
			rf.me, peerIndex, rf.nextIndex[peerIndex], rf.log.first().Index, rf.log.nextIndex())
	}

	prevLogIndex := rf.SnapShot.LastIncludedIndex
	prevLogTerm := rf.SnapShot.LastIncludedTerm

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
	args.TraceId = getNextTraceId()
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
	//log.Printf("inst %d: AE Resp: inst %d reply success: %v", rf.me, peerIndex, reply.Success)

	if reply.Term > rf.currentTerm {
		log.Printf("inst %d: AE Resp: leader becomes follower because new leader %d with higher term: %d -> %d",
			rf.me, peerIndex, rf.currentTerm, reply.Term)
		rf.currentTerm = reply.Term
		rf.votedFor = -1
		rf.electionTimeoutAt = getNextElectionTimeout()
		rf.state = STATE_FOLLOWER
		rf.persist()
	}

	if rf.state != STATE_LEADER { // only leader needs to handle AppendEntries RPC reply
		return
	}

	if args.Term != reply.Term {
		return
	}

	if reply.Success {
		// we need to guard against old reply in the same term, which acknowledged old (smaller) matchIndex
		// because matchIndex should not go back
		if prevLogIndex+len(entries) > rf.matchIndex[peerIndex] {
			log.Printf("inst %d: AE Resp: leader updates matchIndex[%d]: %d -> %d",
				rf.me, peerIndex, rf.matchIndex[peerIndex], prevLogIndex+len(entries))
			rf.matchIndex[peerIndex] = prevLogIndex + len(entries)
			if LOG_BACKTRACKING_MODE != LOG_BT_AGGRESSIVE {
				rf.nextIndex[peerIndex] = rf.matchIndex[peerIndex] + 1
			}
		}
		rf.successiveLogConflict[peerIndex] = SUCCESSIVE_CONFLICT_OFFSET
	} else {
		// TODO: fix this mode
		if LOG_BACKTRACKING_MODE == LOG_BT_TERM_BYPASS {
			i := -1
			if !rf.log.isEmpty() {
				i = rf.log.last().Index
				flag := true
				for ; i > rf.log.first().Index; i-- {
					if rf.log.get(i).Term == reply.ConflictTerm {
						flag = false
						break
					}
				}
				if flag {
					i = -1
				}
			}

			if i >= rf.log.first().Index {
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

	// TODO: is this line necessary?
	//rf.nextIndex[peerIndex] = max(rf.nextIndex[peerIndex], 1)

	// rf.firstLogIndexCurrentTerm ensures that log[N].term == currentTem
	// try to find an N s.t. N > commitIndex, so just let N = commitIndex + 1 first, then N++
	N := max(rf.commitIndex+1, rf.firstLogIndexCurrentTerm)
	for {
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
				log.Fatalf("inst %d: AE Resp: Error: the newly committed log's term is %d, but current term is %d. commitIndex: %d",
					rf.me, rf.log.get(N).Term, currentTerm, rf.commitIndex)
			}
			log.Printf("inst %d: AE Resp: leader commits new logs (commitIndex: %d) at term %d",
				rf.me, rf.commitIndex, rf.currentTerm)
		} else {
			break
		}
		N++
	}

	// fast retry
	if !reply.Success && conflictRetries > 0 {
		go rf.sendAERequestAndHandleReply(peerIndex, conflictRetries-1)
	}
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	reply.TraceId = args.TraceId
	reply.Success = true
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

	log.Printf("inst %d: AE Req: arg PrevLogIndex: %d, PrevLogTerm: %d, log len %d",
		rf.me, args.PrevLogIndex, args.PrevLogTerm, len(args.Entries))

	// #2
	if rf.SnapShot.LastIncludedIndex == args.PrevLogIndex &&
		rf.SnapShot.LastIncludedTerm == args.PrevLogTerm {
		// reply should succeed in this case
	} else if entry := rf.log.get(args.PrevLogIndex); entry == nil || entry.Term != args.PrevLogTerm {
		reply.Success = false
		//log.Printf("inst %d: AE Req: log len: %d", rf.me, len(rf.log.Entries))
	}

	log.Printf("inst %d: AE Req: reply success: %v", rf.me, reply.Success)

	if !reply.Success {
		// TODO: fix this mode
		if LOG_BACKTRACKING_MODE == LOG_BT_TERM_BYPASS {
			entry := rf.log.get(args.PrevLogIndex)
			if entry == nil {
				reply.ConflictTerm = -1
				reply.ConflictIndex = 1 // send snapshot in the next round
			} else if entry.Term != args.PrevLogTerm {
				reply.ConflictTerm = entry.Term

				if rf.SnapShot.LastIncludedTerm == reply.ConflictTerm {
					// first index of conflictTerm is compacted in the snapshot already
					reply.ConflictTerm = -1
					reply.ConflictIndex = 1 // send snapshot in the next round
				} else {
					// the student's guide does not say to store the search result into ConflictIndex.
					// But I guess we should do this
					reply.ConflictIndex = args.PrevLogIndex
					for reply.ConflictIndex-1 >= rf.log.first().Index &&
						rf.log.get(reply.ConflictIndex-1).Term == reply.ConflictTerm {
						reply.ConflictIndex--
					}
				}
			}
			log.Printf("inst %d: AE Req: conflictIndex: %d, conflictTerm: %d",
				rf.me, reply.ConflictIndex, reply.ConflictTerm)
		}

		return
	}

	// #3
	i := args.PrevLogIndex + 1
	j := 0
	for !rf.log.isEmpty() && i < rf.log.nextIndex() && j < len(args.Entries) {
		if rf.log.get(i).Term != args.Entries[j].Term { // conflict happens.
			rf.log.Entries = rf.log.getRange(rf.log.first().Index, i-1) // delete existing entries
			//i = rf.log.nextIndex()
			if i <= rf.commitIndex {
				log.Fatalf("inst %d: AE Req: follower commitIndex %d, but leader %d trys to overwrite log beginning at index %d",
					rf.me, rf.commitIndex, args.LeaderId, i)
			}
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
		log.Printf("inst %d: AE Req: follower appends log entires (last entry index %d)",
			rf.me, rf.lastNewEntryIndex)
	}

	// #5
	if args.LeaderCommit > rf.commitIndex && rf.lastNewEntryIndex != -1 {
		rf.commitIndex = min(args.LeaderCommit, rf.lastNewEntryIndex)
		log.Printf("inst %d: AE Req: follower commits new logs (commitIndex: %d) at term %d",
			rf.me, rf.commitIndex, rf.currentTerm)
	}
}

// This function must be called in the critical section
func (rf *Raft) sendHeartbeats() {
	for i := range rf.peers {
		if i == rf.me {
			continue
		}
		go rf.sendAERequestAndHandleReply(i, AE_CONFLICT_RETRIES)
	}
}

func getNextHeartbeatTime() time.Time {
	return time.Now().Add(HEARTBEAT_FERQUENCY)
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
