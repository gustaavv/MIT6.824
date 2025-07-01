package atopraft

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type clientSession struct {
	mu       sync.Mutex
	cid      int
	lastXid  int
	lastResp *SrvReply
	condMu   sync.Mutex
	cond     *sync.Cond
}

func (cs *clientSession) String(config *BaseConfig) string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return fmt.Sprintf("CS{Cid: %d, lastXid: %d, lastRespValue: %q}",
		cs.cid, cs.lastXid, LogV(cs.lastResp.Value.(ReplyValue), config))
}

func (cs *clientSession) getLastXidAndResp() (lastXid int, lastResp SrvReply) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	lastXid = cs.lastXid
	lastResp = *cs.lastResp
	return
}

func (cs *clientSession) setLastXidAndResp(lastXid int, lastResp SrvReply) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.lastXid = lastXid
	cs.lastResp = &lastResp
}

func makeClientSession(cid int) *clientSession {
	cs := new(clientSession)

	cs.cid = cid
	cs.cond = sync.NewCond(&cs.condMu)
	cs.lastResp = new(SrvReply)

	return cs
}

// session This object works for the same purpose as that of Session in Java EE
type session struct {
	mu sync.Mutex
	// id -> clientSession
	clientSessionMap map[int]*clientSession
}

func (s *session) String(config *BaseConfig) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Session{csNum: %d, csMap: {", len(s.clientSessionMap)))
	csList := make([]*clientSession, 0)
	for _, cs := range s.clientSessionMap {
		csList = append(csList, cs)
	}
	sort.Slice(csList, func(i, j int) bool {
		return csList[i].cid < csList[j].cid
	})
	for _, cs := range csList {
		sb.WriteString(cs.String(config))
		sb.WriteString(", ")
	}
	sb.WriteString("}}")
	return sb.String()
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
