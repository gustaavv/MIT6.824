package raft

import (
	"log"
	"math/rand"
	"sync"
	"time"
)

// RequestVoteArgs
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// RequestVoteReply
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}

// RequestVote
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()

	// #1
	if args.Term < rf.currentTerm { // outdated candidate
		reply.Term = rf.currentTerm
		reply.VoteGranted = false
		return
	}

	reply.Term = args.Term

	if args.Term > rf.currentTerm {
		log.Printf("inst %d: RV Req: %v becomes follower because candidate (inst %d) with higher term: %d -> %d",
			rf.me, rf.state, args.CandidateId, rf.currentTerm, args.Term)
		rf.currentTerm = args.Term
		rf.votedFor = -1
		rf.electionTimeoutAt = getNextElectionTimeout()
		rf.state = STATE_FOLLOWER
	}

	// #2
	voterLastLogIndex := rf.log.SnapShot.LastIncludedIndex
	voterLastLogTerm := rf.log.SnapShot.LastIncludedTerm

	if !rf.log.isEmpty() {
		voterLastLogEntry := rf.log.last()
		voterLastLogIndex = voterLastLogEntry.Index
		voterLastLogTerm = voterLastLogEntry.Term
	}

	candidateLogUpToDate := false
	if args.LastLogTerm != voterLastLogTerm {
		candidateLogUpToDate = args.LastLogTerm >= voterLastLogTerm
	} else {
		candidateLogUpToDate = args.LastLogIndex >= voterLastLogIndex
	}

	if (rf.votedFor == -1 || rf.votedFor == args.CandidateId) && candidateLogUpToDate {
		reply.VoteGranted = true
		rf.votedFor = args.CandidateId
		// Reset election timeout when "granting vote to candidate",
		// otherwise the follower will never become a candidate.
		rf.electionTimeoutAt = getNextElectionTimeout()
	}
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func getElectionTimeoutDuration() time.Duration {
	minMs := ELECTION_TIMEOUT_MIN_TIME_MS
	maxMs := ELECTION_TIMEOUT_MAX_TIME_MS
	timeoutMs := minMs + rand.Intn(maxMs-minMs)
	return time.Duration(timeoutMs)
}

func getNextElectionTimeout() time.Time {
	return time.Now().Add(getElectionTimeoutDuration() * time.Millisecond)
}

// The electionTimeoutTicker go routine starts a new election if this peer hasn't received
// heartbeats recently.
func (rf *Raft) electionTimeoutTicker() {
	for rf.killed() == false {

		// Your code here to check if a leader election should
		// be started and to randomize sleeping time using
		// time.Sleep().

		// Although time.Sleep() can not timeout at exactly rf.electionTimeoutAt,
		// it is significantly simpler using time.Sleep() than using time.ticker().
		time.Sleep(TICKER_FREQUENCY)

		rf.mu.Lock()

		if rf.state != STATE_FOLLOWER { // only follower has election timeout
			rf.mu.Unlock()
			continue
		}

		if rf.electionTimeoutAt.After(time.Now()) { // not timeout yet
			rf.mu.Unlock()
			continue
		}

		// start election as candidate
		rf.state = STATE_CANDIDATE
		rf.currentTerm++
		rf.votedFor = rf.me
		rf.persist()
		log.Printf("inst %d: ticker: follower becomes candidate at a new term %d", rf.me, rf.currentTerm)

		timeout := time.After(getElectionTimeoutDuration() * time.Millisecond)

		// Get a majority vote is enough to be elected as the new leader, but a waitGroup can not
		// Done() more than Add(), otherwise there is a 'panic: sync: negative WaitGroup counter'.
		// So we need to use a counter to prevent this from happening.
		voteCount := new(int)
		*voteCount = len(rf.peers) / 2
		var voteCountMutex sync.Mutex
		var wg sync.WaitGroup
		wg.Add(*voteCount)
		electionSuccess := make(chan struct{})

		go func() {
			wg.Wait()
			close(electionSuccess)
		}()

		term := rf.currentTerm
		candidateId := rf.me
		lastLogEntry := rf.log.last()
		lastLogIndex := lastLogEntry.Index
		lastLogTerm := lastLogEntry.Term

		for i, peer := range rf.peers {
			if i == rf.me {
				continue
			}

			peer := peer
			go func() {
				args := new(RequestVoteArgs)
				args.Term = term
				args.CandidateId = candidateId
				args.LastLogIndex = lastLogIndex
				args.LastLogTerm = lastLogTerm

				reply := new(RequestVoteReply)
				ok := peer.Call("Raft.RequestVote", args, reply)

				if !ok { // network failure
					return
				}

				// verify reply
				rf.mu.Lock()
				defer rf.mu.Unlock()

				if rf.state != STATE_CANDIDATE { // only candidate needs to handle RequestVote RPC reply
					return
				}

				if reply.Term > rf.currentTerm {
					log.Printf("inst %d: RV Resp: candidate becomes follower because higher term: %d -> %d",
						rf.me, rf.currentTerm, reply.Term)
					rf.currentTerm = reply.Term
					rf.votedFor = -1
					rf.electionTimeoutAt = getNextElectionTimeout()
					rf.state = STATE_FOLLOWER
					rf.persist()
					return
				}

				// the term should be the same to be a valid vote
				if args.Term == reply.Term && reply.VoteGranted {
					voteCountMutex.Lock()
					if *voteCount > 0 {
						*voteCount--
						wg.Done()
					}
					voteCountMutex.Unlock()
				}
			}()
		}

		rf.mu.Unlock()

		select {
		case <-timeout:
			rf.mu.Lock()

			if rf.state != STATE_CANDIDATE {
				rf.mu.Unlock()
				continue
			}

			log.Printf("inst %d: ticker: candidate election at term %d timeouts (lacking %d votes). Start a new one",
				rf.me, term, *voteCount)

			// Although it shouldn't be a follower as in Figure 2, but for the convenience of coding, let's do this.
			// Since we set election timeout at now, it won't wait too much time
			rf.electionTimeoutAt = time.Now()
			rf.state = STATE_FOLLOWER
			rf.votedFor = -1
			rf.persist()

			rf.mu.Unlock()

		case <-electionSuccess:
			rf.mu.Lock()

			if rf.state != STATE_CANDIDATE {
				rf.mu.Unlock()
				continue
			}

			log.Printf("inst %d: ticker: candidate becomes leader at term %d", rf.me, rf.currentTerm)
			rf.state = STATE_LEADER
			rf.initVolatileLeaderState()
			rf.firstLogIndexCurrentTerm = rf.log.nextIndex()
			rf.heartbeatAt = time.Now()

			// send heartbeat immediately after becoming a leader
			rf.sendHeartbeats()

			rf.mu.Unlock()
		}
	}
}
