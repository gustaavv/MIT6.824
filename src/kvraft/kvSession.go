package kvraft

import "sync"

type clientSession struct {
	mu       sync.Mutex
	cid      int
	lastXid  int
	lastResp *KVReply
	condMu   sync.Mutex
	cond     *sync.Cond
}

func (cs *clientSession) getLastXidAndResp() (lastXid int, lastResp KVReply) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	lastXid = cs.lastXid
	lastResp = *cs.lastResp
	return
}

func (cs *clientSession) setLastXidAndResp(lastXid int, lastResp KVReply) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.lastXid = lastXid
	cs.lastResp = &lastResp
}

func makeClientSession(cid int) *clientSession {
	cs := new(clientSession)

	cs.cid = cid
	cs.cond = sync.NewCond(&cs.condMu)
	cs.lastResp = new(KVReply)

	return cs
}

// session This object works for the same purpose as that of Session in Java EE
type session struct {
	mu sync.Mutex
	// id -> clientSession
	clientSessionMap map[int]*clientSession
}

// create a new one if it does not exist
func (s *session) getClientSession(cid int) *clientSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.clientSessionMap[cid]
	if !ok {
		c = makeClientSession(cid)
		s.clientSessionMap[cid] = c
	}
	return c
}

func (s *session) broadcastAllClientSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, cs := range s.clientSessionMap {
		cs.condMu.Lock()
		cs.cond.Broadcast()
		cs.condMu.Unlock()
	}
}
