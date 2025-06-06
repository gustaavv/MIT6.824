package raft

import (
	"log"
	"time"
)

func getNextApplyLogEntryTime() time.Time {
	return time.Now().Add(APPLY_LOGENTRY_FREQUENCY)
}

func (rf *Raft) applyLogEntryTicker(applyCh chan ApplyMsg) {
	for rf.killed() == false {
		time.Sleep(TICKER_FREQUENCY)

		var applyMsg *ApplyMsg = nil

		rf.mu.Lock()
		if rf.applyLogEntryAt.After(time.Now()) { // no need to apply log entry now
			rf.mu.Unlock()
			continue
		}

		if rf.commitIndex > rf.lastApplied {
			rf.lastApplied++
			applyMsg = &ApplyMsg{
				CommandValid: true,
				CommandIndex: rf.lastApplied,
				Command:      rf.log[rf.lastApplied].Command,
			}
		}

		rf.applyLogEntryAt = getNextApplyLogEntryTime()
		rf.mu.Unlock()

		// this channel may block, so we need to send applyMsg outside the
		// critical section to prevent holding the lock for too long
		if applyMsg != nil {
			applyCh <- *applyMsg
			log.Printf("inst %d: ticker2: applied log index: %v", rf.me, applyMsg.CommandIndex)
		}
	}
}
