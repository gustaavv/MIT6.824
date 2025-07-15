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
	value := "nil"
	if cs.lastResp.Value != nil {
		value = LogV(cs.lastResp.Value.(ReplyValue), config)
	}
	return fmt.Sprintf("CS{Cid: %d, lastXid: %d, lastRespValue: %q}",
		cs.cid, cs.lastXid, value)
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

type ClientSessionTemp struct {
	Cid      int
	LastXid  int
	LastResp SrvReply
}

func (cst *ClientSessionTemp) makeClientSession() *clientSession {
	cs := makeClientSession(cst.Cid)
	cs.lastXid = cst.LastXid
	var value interface{} = nil
	if cst.LastResp.Value != nil {
		value = cst.LastResp.Value.(ReplyValue).Clone()
	}
	cs.lastResp = &SrvReply{
		Success: cst.LastResp.Success,
		Msg:     cst.LastResp.Msg,
		Value:   value,
	}
	return cs
}

func (s *session) Clone() []ClientSessionTemp {
	clientSessionList := make([]ClientSessionTemp, 0)
	s.mu.Lock()
	for _, cs := range s.clientSessionMap {
		var value interface{} = nil
		if cs.lastResp.Value != nil {
			value = cs.lastResp.Value.(ReplyValue).Clone()
		}
		clientSessionList = append(clientSessionList, ClientSessionTemp{
			Cid:     cs.cid,
			LastXid: cs.lastXid,
			LastResp: SrvReply{
				Success: cs.lastResp.Success,
				Msg:     cs.lastResp.Msg,
				Value:   value,
			},
		})
	}
	s.mu.Unlock()
	return clientSessionList
}

func (s *session) Update(clientSessionList []ClientSessionTemp) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, cst := range clientSessionList {
		cs, ok := s.clientSessionMap[cst.Cid]
		if ok {
			if xid, _ := cs.getLastXidAndResp(); xid < cst.LastXid {
				csCopy := cst.makeClientSession() // mainly to copy lastResp
				cs.setLastXidAndResp(csCopy.lastXid, *csCopy.lastResp)
			}
		} else {
			s.clientSessionMap[cst.Cid] = cst.makeClientSession()
		}
	}
}
