package raft

import (
	"fmt"
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
	if rf.state != STATE_LEADER {
		rf.mu.Unlock()
		return
	}
	traceId := getNextTraceId()
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

	//firstIndex, nextIndex := -1, -1
	//if !rf.log.isEmpty() {
	//	firstIndex, nextIndex = rf.log.first().Index, rf.log.nextIndex()
	//}
	//log.Printf("inst %d: AE Req: Trace: %d: inst %d's nextIndex: %d. log first entry index: %d, log next index: %d",
	//	rf.me, traceId, peerIndex, rf.nextIndex[peerIndex], firstIndex, nextIndex)

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
	args.TraceId = traceId
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

	logHeader := fmt.Sprintf("inst %d: AE Resp: Trace: %d: ", rf.me, reply.TraceId)

	//log.Printf("%sinst %d reply success: %v", logHeader, peerIndex, reply.Success)

	if reply.Term > rf.currentTerm {
		log.Printf("%s%s becomes follower because inst %d with higher term: %d -> %d",
			logHeader, rf.state, peerIndex, rf.currentTerm, reply.Term)
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
			//log.Printf("%sleader updates matchIndex[%d]: %d -> %d",
			//	logHeader, peerIndex, rf.matchIndex[peerIndex], prevLogIndex+len(entries))
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

	rf.nextIndex[peerIndex] = max(rf.nextIndex[peerIndex], 1)

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
				log.Fatalf("%sError: the newly committed log's term is %d, but current term is %d. commitIndex: %d",
					logHeader, rf.log.get(N).Term, currentTerm, rf.commitIndex)
			}
			log.Printf("%sleader commits new logs (commitIndex: %d) at term %d",
				logHeader, rf.commitIndex, rf.currentTerm)
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
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()
	logHeader := fmt.Sprintf("inst %d: AE Req: Trace: %d: ", rf.me, args.TraceId)

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
		log.Printf("%s%v becomes follower because leader (inst %d) with higher term: %d -> %d",
			logHeader, rf.state, args.LeaderId, rf.currentTerm, args.Term)
		rf.currentTerm = args.Term
		rf.votedFor = -1
		rf.state = STATE_FOLLOWER
	}

	//log.Printf("inst %d: AE Req: arg PrevLogIndex: %d, PrevLogTerm: %d, log len %d",
	//	rf.me, args.PrevLogIndex, args.PrevLogTerm, len(args.Entries))

	// #2

	if args.PrevLogIndex == rf.SnapShot.LastIncludedIndex {
		reply.Success = args.PrevLogTerm == rf.SnapShot.LastIncludedTerm
		if !reply.Success {
			log.Fatalf("%sargs.PrevLogTerm %d != snapShot.LastIncludedTerm %d",
				logHeader, args.PrevLogTerm, rf.SnapShot.LastIncludedTerm)
		}
	} else if args.PrevLogIndex < rf.SnapShot.LastIncludedIndex {
		reply.Success = rf.SnapShot.LastIncludedTerm <= args.PrevLogTerm
		if reply.Success {
			i := 0
			for i < len(args.Entries) && args.Entries[i].Index <= rf.SnapShot.LastIncludedIndex {
				i++
			}

			oldFirstIndex := -1
			if len(args.Entries) > 0 {
				oldFirstIndex = args.Entries[0].Index
			}
			newFirstIndex := -1
			if i < len(args.Entries) {
				newFirstIndex = args.Entries[i].Index
			}
			log.Printf("%strim(start) arg entries: len %d -> %d, first log index %d -> %d",
				logHeader, len(args.Entries), len(args.Entries)-i, oldFirstIndex, newFirstIndex)

			args.Entries = args.Entries[i:]
			args.PrevLogIndex = rf.SnapShot.LastIncludedIndex
			args.PrevLogTerm = rf.SnapShot.LastIncludedTerm
		} else {
			log.Printf("%sarg.prevLogIndex %d, arg.prevLogTerm %d, snapShot.LastIncludedIndex %d, snapShot.LastIncludedTerm %d",
				logHeader, args.PrevLogIndex, args.PrevLogTerm, rf.SnapShot.LastIncludedIndex, rf.SnapShot.LastIncludedTerm)
		}
	} else {
		entry := rf.log.get(args.PrevLogIndex)
		reply.Success = !(entry == nil || entry.Term != args.PrevLogTerm)
	}

	if len(args.Entries) > 0 && args.Entries[0].Index <= rf.SnapShot.LastIncludedIndex {
		log.Fatalf("%sarg first entry index %d <= snapShot.LastIncludedIndex %d",
			logHeader, args.Entries[0].Index, rf.SnapShot.LastIncludedIndex)
	}

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
			log.Printf("%sconflictIndex: %d, conflictTerm: %d",
				logHeader, reply.ConflictIndex, reply.ConflictTerm)
		}

		return
	}

	// #3
	i := args.PrevLogIndex + 1
	j := 0
	for !rf.log.isEmpty() && i < rf.log.nextIndex() && j < len(args.Entries) {
		if rf.log.get(i).Term != args.Entries[j].Term { // conflict happens.
			rf.log.Entries = rf.log.getRange(rf.log.first().Index, i-1) // delete existing entries
			if i <= rf.commitIndex {
				log.Fatalf("%sfollower commitIndex %d, but leader %d trys to overwrite log beginning at index %d",
					logHeader, rf.commitIndex, args.LeaderId, i)
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
		log.Printf("%sfollower appends log entires (last entry index %d)",
			logHeader, rf.lastNewEntryIndex)
	}

	// #5
	if args.LeaderCommit > rf.commitIndex && rf.lastNewEntryIndex != -1 {
		rf.commitIndex = min(args.LeaderCommit, rf.lastNewEntryIndex)
		log.Printf("%sfollower commits new logs (commitIndex: %d) at term %d",
			logHeader, rf.commitIndex, rf.currentTerm)
	}
}

func getNextHeartbeatTime() time.Time {
	return time.Now().Add(HEARTBEAT_FERQUENCY)
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

		rf.sendHeartbeats()
		rf.heartbeatAt = getNextHeartbeatTime()

		rf.mu.Unlock()
	}
}
