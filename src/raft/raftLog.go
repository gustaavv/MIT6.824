package raft

import "log"

type LogEntry struct {
	Term    int
	Command interface{}
	Index   int
}

type raftLog struct {
	Entries []LogEntry
}

func (rl *raftLog) size() int {
	return len(rl.Entries)
}

func (rl *raftLog) isEmpty() bool {
	return rl.size() == 0
}

func (rl *raftLog) first() LogEntry {
	return rl.Entries[0]
}

func (rl *raftLog) last() LogEntry {
	return rl.Entries[len(rl.Entries)-1]
}

func (rl *raftLog) nextIndex() int {
	return rl.last().Index + 1
}

func (rl *raftLog) checkValidIndex(index int) bool {
	firstIndex := rl.first().Index
	lastIndex := rl.last().Index
	ans := firstIndex <= index && index <= lastIndex
	if !ans {
		//log.Printf("index out of range: %d not in [%d, %d]", index, firstIndex, lastIndex)
	}

	return ans
}

// return nil if index is out of range
func (rl *raftLog) get(index int) *LogEntry {
	if rl.isEmpty() {
		return nil
	}
	firstIndex := rl.first().Index

	if !rl.checkValidIndex(index) {
		return nil
	}
	// indices are continuous
	return &rl.Entries[index-firstIndex]
}

// both indices are inclusive. Return nil if either index is out of range
func (rl *raftLog) getRange(startIndex int, endIndex int) []LogEntry {
	firstIndex := rl.first().Index
	if !rl.checkValidIndex(startIndex) {
		return nil
	}
	if !rl.checkValidIndex(endIndex) {
		return nil
	}
	if startIndex > endIndex {
		log.Panicf("startIndex(%d) > endIndex(%d)", startIndex, endIndex)
	}
	// indices are continuous
	return rl.Entries[startIndex-firstIndex : endIndex-firstIndex+1]
}

func (rl *raftLog) getRangeStartFrom(startIndex int) []LogEntry {
	firstIndex := rl.first().Index
	if !(firstIndex <= startIndex && startIndex <= rl.nextIndex()) {
		return nil
	}
	return rl.Entries[startIndex-firstIndex:]
}

func (rl *raftLog) append(entries ...LogEntry) {
	rl.Entries = append(rl.Entries, entries...)
}
