# Lab 2D: Raft log compaction

http://nil.csail.mit.edu/6.824/2021/labs/lab-raft.html#:~:text=Part%202D:%20log%20compaction


## Design

### Refactor: log entry index

Before lab 2D, we are used to using index of `log[]` as the log index. But as mentioned in Hint #3, we need to refactor our code to find another way to store log indices as well as get them before starting to implement the snapshot mechanism. I add a `Index` field to `LogEntry` to achieve this.

### InstallSnapshot RPC

The snapshot mechanism requires a new RPC handler.

When the leader wants to send an AppendEntries RPC request, if `nextIndex` is no greater than the snapshot's `LastIncludedIndex`, which means the follower is lagging behind, an InstallSnapshot RPC request will be sent instead.

### Optimization

1️⃣ After state machine calls `Start()`, the leader sends AppendEntries RPC requests immediately instead of relying on the heartbeat to send the newly appended log entry.

2️⃣ Fast retry when an AppendEntries RPC reply suggests that there is a conflict in log.

3️⃣ Add a schedule job that do lock and unlock every 1 second just to check if there is a dead lock.

## Experiences

### Difficulty

Refactoring takes efforts, because Figure 2 is based on using index of `log[]` as the log index. We have nothing to follow but have to work out our own solution (cover all the corner cases).

This lab is very hard in terms of difficulty. Despite the fact that Figure 13 probably gives detailed instructions, nobody tells us how to combine Figure 2 and Figure 13, so we must figure out all the corner cases after adding the snapshot mechanism to the existing system. Let alone the code becomes nasty (hard to understand) as a result of covering those corner cases.

It may sounds like I am unreflecting and happy to follow other's instructions. But the main concern is whether adding a new feature will maintain the correctness of the Raft protocol. Unfortunately, I don't how to prove that.

### Log used in debug

When stuck with a bug for a long time, the built-in log package gets unhandy, because it does not support log level and the log we add to find that bug belongs to DEBUG level. After successful debugging, those log should be turned off (commented out). 

But I am also not really interested in installing a production-ready log framework, which takes efforts to config. The main reason is that I think I need something like a log with feature flags when debugging, not a log with different levels.

Since I just read through the logs to identify bugs, I deliberately not to log too much, which could be a bad idea because it is suggested to log every RPC requests and replies. I am also not interested in building a tool to parse the long log, like the one mentioned at the beginning of lecture 8.

### Log backtracking
In terms of [log backtracking mode](./lab2b.md#Log%20backtracking), Conflict Term Bypassing is the best in terms of time performance before lab 2D, but it needs additional refactoring after implementing the snapshot mechanism. So, I switched to Binary Exponential in order to get the snapshot mechanism right when implementing it. Although this mode will sometimes time out (a 2-min limit on every 2D test), it is important to get things right first and after that do we consider optimization. This reflects separation of concern in coding.

After trying to refactor Conflict Term Bypassing for a while, I gave up. The main reason is that I don't fully understand the instructions in the [Students' Guide to Raft](https://thesquareplanet.com/blog/students-guide-to-raft/#:~:text=accelerated%20log%20backtracking%20optimization), so tailoring it to the snapshot mechanism is very difficult. 

A workaround is to upgrade Binary Exponential: the initial value in `successiveLogConflict` is set to 3 instead of 0. Intuitively, this initial value represents the number of AppendEntries RPC round trip we can save when there is a lagging follower. However, setting the value to 3 is kind of intentional, because in `snapcommon()` of `test_test.go`, there are 11 `Start()` being called in each iteration, during which a server may be disconnected or crashed. 

But I think it is ok because the state machine will do snapshot quite often, and therefore whatever log backtracking mode we are using, we will inevitably send InstallSnapshot RPC request quickly after several failed AppendEntries RPC requests. Moreover, the test result turns out to be quite good.

### An annoying bug

> I have been stuck in this bug for a long time, so I decide to record it.

The bug: after implementing AppendEntries RPC fast retry, inconsistency happens. The way I implement fast retry is to let `sendAERequestAndHandleReply()` call itself if the request fails. However, if the reply indicates that this leader is at an older term, it should not send a new request. Otherwise, there will be a split-brain syndrome.

Of course, I didn't realize it was the fast retry that caused this bug at first. I thought it was the snapshot mechanism. But 2D tests never failed, while `TestFigure8Unreliable2C` failed very often, which doesn't involve snapshot or crash, just random delay and disconnect. This is weird, as the new feature only leads to failure when this new feature is not enabled.

So, I added a lot log to try to identify that bug. In the end, I find adding a globally incremental trace ID to each request helps show that a follower, which previously was a leader, is sending AppendEntries RPC requests, causing inconsistency.

The solution is simple: check the instance's state before sending AppendEntries request or InstallSnapshot request, both of which requires leader "privilege" to send.

I think it is the last missing code to fully pass tests before lab 2D. In previous labs, I did see some test failures, but I kind of "naturally" ignore them, thinking perhaps the system instead of my code went wrong.

Before:
```go
func (rf *Raft) sendAERequestAndHandleReply(peerIndex int) {
	rf.mu.Lock()
	// copy raft state into args
	args := new(AppendEntriesArgs)
	args.Term = rf.currentTerm
	rf.mu.Unlock()
	
	reply := new(AppendEntriesReply)
	// send RPC request
	ok := peer.Call("Raft.AppendEntries", args, reply)
	
	if !ok { // network failure
		return
	}
	
	rf.mu.Lock()
	defer rf.mu.Unlock()
	
	// first check leadership
	// then handle reply
}
```

Now: 

```go
func (rf *Raft) sendAERequestAndHandleReply(peerIndex int) {
	rf.mu.Lock()
	// check leadership first
	if rf.state != STATE_LEADER {
		rf.mu.Unlock()
		return
	}	
	// copy raft state into args
	args := new(AppendEntriesArgs)
	args.Term = rf.currentTerm
	rf.mu.Unlock()
	
	reply := new(AppendEntriesReply)
	// send RPC request
	ok := peer.Call("Raft.AppendEntries", args, reply)
	
	if !ok { // network failure
		return
	}
	
	rf.mu.Lock()
	defer rf.mu.Unlock()
	
	// first check leadership again
	// then handle reply
}
```

What if the raft instance is deposed to a follower after copying raft state into args and before sending RPC request? The whole system is still correct, because this request has an outdated term. The only drawback is the wasted network bandwidth to send this packet.

How to solve this problem? We actually can't, because `peer.Call()` is a synchronous call, and we can't call this function inside a critical section. If there is an asynchronous version, say `CallAsync()`, we can do something like this:

```go
func (rf *Raft) sendAERequestAndHandleReply(peerIndex int) {
	rf.mu.Lock()
	// check leadership first
	if rf.state != STATE_LEADER {
		rf.mu.Unlock()
		return
	}	
	// copy raft state into args
	args := new(AppendEntriesArgs)
	args.Term = rf.currentTerm
	
	reply := new(AppendEntriesReply)
	// send RPC request asynchronously
	ch := peer.CallAsync("Raft.AppendEntries", args, reply)
	rf.mu.Unlock()
	
	// CallAsync() works like Go rpc package's client.Go() function
	<-ch.Done
	
	rf.mu.Lock()
	defer rf.mu.Unlock()
	
	// first check leadership again
	// then handle reply
}
```

Although the `labrpc` package does not provide such function, it is ok because the resulting overhead is very little. I am just showing a theoretically best-performant code.