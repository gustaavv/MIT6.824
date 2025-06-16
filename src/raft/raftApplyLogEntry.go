package raft

import (
	"fmt"
	"log"
	"time"
)

func (rf *Raft) applyLogEntryTicker() {
	logHeader := fmt.Sprintf("inst %d: ticker2: ", rf.me)
	lastRound := time.Now()
	for rf.killed() == false {
		//time.Sleep(APPLY_LOGENTRY_FREQUENCY)

		var applyMsg *ApplyMsg = nil
		var instState = ""

		rf.mu.Lock()

		if rf.commitIndex > rf.lastApplied {
			rf.lastApplied++
			//rf.lastAppliedPersist = max(rf.lastApplied, rf.lastAppliedPersist)
			//rf.persist()

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
		} else {
			rf.mu.Unlock()
			rf.applyLogEntryMu.Lock()
			rf.applyLogEntryCond.Wait()
			rf.applyLogEntryMu.Unlock()
			continue
		}

		rf.mu.Unlock()

		// this channel may block, so we need to send applyMsg outside the
		// critical section to prevent holding the lock for too long
		if applyMsg != nil {
			rf.applyCh <- *applyMsg
			log.Printf("%s%s applied log index: %v", logHeader, instState, applyMsg.CommandIndex)
			log.Printf("%s%.3f seconds passed since last applied", logHeader, time.Since(lastRound).Seconds())
			lastRound = time.Now()
		}

	}
}
