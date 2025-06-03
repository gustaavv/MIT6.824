# Lab 2B: Raft log replication

http://nil.csail.mit.edu/6.824/2021/labs/lab-raft.html#:~:text=Part%202B:%20log

## Design

### AppendEntries RPC

According to [this visualization tutorial](https://thesecretlivesofdata.com/raft/#replication), the leader does not send AppendEntries RPC request proactively to replicate new client commands, but only at heartbeat. In this way, the follower will commit the new log entries at the 2nd heartbeat after a new client command sent to the leader if everything goes well.

### `applyLogEntryTicker`

Add another schedule job: applying committed log entries for all servers.

Do not send to a channel inside a critical section. When the channel blocks, the goroutine will hold the lock for too long, causing the whole system halting if that lock is used very often. See details on how to do this in the function.

What matters is the frequency of running this schedule job, which should be small enough, e.g. 10ms. This is necessary because only one committed log entry will be applied in each run, while the tester expects committing and applying a log entry to happen at nearly the same time with a total timeout of 10s. In `TestBackup2B`, when a disconnected follower rejoins, it will have more than 50 log entries to apply, which will cause tester failure because of timeout if the frequency is large.

It is ok to set this frequency small enough, because the critical section is non-blocking.

Maintaining the non-blocking implementation, another optimization can be batching the log entries to be applied. For example, take the next 10 log entires committed but not applied into a slice. After exiting the critical section, traverse the slice and send each log entry to `applyCh`. But I am concerned about the potential inconsistency brought by this optimization, so I didn't adopt it.

### Log backtracking

Besides the frequency of applying log entries, `TestBackup2B` requires more than original log backtracking strategy in Figure 2. This test is the bottleneck when try to follow the rule 'use no more than a minute of real time for the 2B tests'.

There are 4 log backtracking modes provided in the code. You can switch the feature flag `LOG_BACKTRACKING_MODE` to use each one.

1️⃣ Original

```go
if !AppendEntriesReply.Success {
    rf.nextIndex[peerIndex]--
}
```

2️⃣ Binary Exponential

`nextIndex` is decreased by $1, 2, 4, 8, \cdots, 2^n$ when $n$ successive log conflicts happen.

`nextIndex` will be decreased dramatically when $n$ increases. In addition, the number of duplicate entries will only be one time to that of the entries the follower lacks, only causing slight overhead to the network traffic. 

For example, if the follower lacks $17$ entries, AppendEntries request with $16$ entries ($n = 4$) will fail while the one after with $32$ entries ($n = 5$) will succeed. we are only sending $32 - 17 = 15$ duplicate entires, which is less than $17$.

Since Figure 2 enables the ability to handle duplicate entries, we can easily take advantage of this feature to implement this log backtracking mode.

Create an int slice `successiveLogConflict`, whose elements will be set to 0 at every new term.

```go
if !AppendEntriesReply.Success {
    rf.nextIndex[peerIndex] -= 1 << rf.successiveLogConflict[peerIndex]
    rf.nextIndex[peerIndex] = max(rf.nextIndex[peerIndex], 1)
} else {
    rf.successiveLogConflict[peerIndex] = 0
}
```

3️⃣ Conflict Term Bypassing

This is the idea mentioned at the end of section 5.3 of the paper. The [Students' Guide to Raft](https://thesquareplanet.com/blog/students-guide-to-raft/#:~:text=accelerated%20log%20backtracking%20optimization) gave more detailed instructions. It works but the instructions are obscure. Maybe the author should just give pseudocode to make it more understandable.

See the code for detailed implementation. This mode is the hardest in terms of implementation as well as understanding.

4️⃣ Super Aggressive

`nextIndex` is always set to 1 and therefore all the entires will be sent in every AppendEntries request. The time performance is good, but the cost to the network is too large to bear. Also note that we make use of the ability of handling duplication.

`TestRPCBytes2B` will fail if we use this mode.

```
Test (2B): RPC byte count ...
--- FAIL: TestRPCBytes2B (2.96s)
    test_test.go:192: too many RPC bytes; got 1116900, expected 150000
```

---

Here is a table comparing the longest time running `TestBackup2B` using different log backtracking modes.

| Mode                    | Time(s) |
| ----------------------- | ------- |
| Original                | 42      |
| Binary Exponential      | 32      |
| Conflict Term Bypassing | 30      |
| Super Aggressive        | 31      |


## Experiences

### Run test 2a first
After finishing the code of lab 2b, run tests of lab 2a first, which should be passed because only a little more code added to the election part.

### Difficulty

This lab is indeed hard in terms of difficulty compared with lab 2A, which is considered as moderate. The rise in difficulty mainly results from understanding log replication, both the idea (theory) and implementation details, which is significantly harder than understanding leader election. 
  
As in the [Instructors’ Guide to Raft](https://thesquareplanet.com/blog/instructors-guide-to-raft/), Raft is easier to understand than Paxos not because its idea is simpler, but because of Figure 2, the ultimate guide offered by the author. 
  
Leaving the comparison with Paxos aside, I agree with the idea especially when I don't understand the Receiver implementation #3 to #5 of AppendEntries RPC, which I consider the hardest of all. I did not grasp the theory of Raft perfectly, then when I don't understand Figure 2, what should I do? Luckily, [the last question in the Raft Q&A](https://thesquareplanet.com/blog/raft-qa/) where the questioner gave a detailed example is very helpful for me to gain better understanding and to work out a solution. 

Another possible solution is to turn to LLM, but I deliberately not to do so for a few reasons: (a) I want to practice my abilities to read papers; (b) This lab was at 2021, before the first LLM product ChatGPT appeared. If people at that time could do it, I should also do it in the same way; (c) AI may be wrong.

