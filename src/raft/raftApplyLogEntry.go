package raft

import (
	"fmt"
	"log"
	"time"
)

func getNextApplyLogEntryTime() time.Time {
	return time.Now().Add(APPLY_LOGENTRY_FREQUENCY)
}

func (rf *Raft) applyLogEntryTicker() {
	logHeader := fmt.Sprintf("inst %d: ticker2: ", rf.me)
	for rf.killed() == false {
		time.Sleep(TICKER_FREQUENCY)

		var applyMsg *ApplyMsg = nil
		var instState = ""

		rf.mu.Lock()
		if rf.applyLogEntryAt.After(time.Now()) { // no need to apply log entry now
			rf.mu.Unlock()
			continue
		}

		if rf.commitIndex > rf.lastApplied {
			rf.lastApplied++

			if entry := rf.log.get(rf.lastApplied); entry == nil {
				log.Fatalf("%slastApplied: %d, log index range: [%d, %d]",
					logHeader, rf.lastApplied, rf.log.first().Index, rf.log.last().Index)
			}

			applyMsg = &ApplyMsg{
				CommandValid: true,
				CommandIndex: rf.lastApplied,
				Command:      rf.log.get(rf.lastApplied).Command,
			}

			instState = rf.state // copy this field to prevent data race
		}

		rf.applyLogEntryAt = getNextApplyLogEntryTime()
		rf.mu.Unlock()

		// this channel may block, so we need to send applyMsg outside the
		// critical section to prevent holding the lock for too long
		if applyMsg != nil {
			rf.applyCh <- *applyMsg
			log.Printf("%s%s applied log index: %v", logHeader, instState, applyMsg.CommandIndex)
		}
	}
}
