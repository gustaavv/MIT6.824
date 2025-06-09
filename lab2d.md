# Lab 2D: Raft log compaction

http://nil.csail.mit.edu/6.824/2021/labs/lab-raft.html#:~:text=Part%202D:%20log%20compaction


## Design

### Refactor: log entry index

Before lab 2D, we are used to using index of `log[]` as the log index. But as mentioned in Hint #3, we need to refactor our code to find another way to store and get log indices before starting to implement the snapshot mechanism. I add a `Index` field to `LogEntry` to achieve this.

### InstallSnapshot RPC

The snapshot mechanism requires a new RPC handler.

When the leader wants to send an AppendEntries RPC request, if `nextIndex` is no greater than the snapshot's `LastIncludedIndex`, which means the follower is lagging behind, an InstallSnapshot RPC request will be sent instead.

## Experiences

### Difficulty

Refactoring takes efforts, because Figure 2 is based on using index of `log[]` as the log index. We have nothing to follow but have to work out our own solution (cover all the corner cases).

This lab is very hard in terms of difficulty. Despite the fact that Figure 13 probably gives detailed instructions, nobody tells us how to combine Figure 2 and Figure 13, so we must figure out all the corner cases after adding the snapshot mechanism to the existing system. Let alone the code becomes nasty (hard to understand) as a result of covering those corner cases.

### Log used in debug

When stuck with a bug for a long time, the built-in log package gets unhandy, because it does not support log level and the log we add to find that bug belongs to DEBUG level. After successful debugging, those log should be turned off (commented out). 

But I am also not really interested in installing a production-ready log framework, which takes efforts to config. The main reason is that I think I need something like a log with feature flags when debugging, not a log with different levels.

Since I just read through the logs to identify bugs, I deliberately not to log too much, which could be a bad idea because it is suggested to log every RPC requests and replies. I am also not interested in building a tool to parse the long log, like the one mentioned at the beginning of lecture 8.

### Log backtracking
In terms of [log backtracking mode](./lab2b.md#Log%20backtracking), Conflict Term Bypassing is the best in terms of time performance before lab 2D, but it needs additional refactoring after implementing the snapshot mechanism. So, I switched to Binary Exponential in order to get the snapshot mechanism right when implementing it. Although this mode will sometimes time out (a 2-min limit on every 2D test), it is important to get things right first and after that do we consider optimization. This reflects separation of concern in coding.

After trying to refactor Conflict Term Bypassing for a while, I gave up. The main reason is that I don't fully understand the instructions in the [Students' Guide to Raft](https://thesquareplanet.com/blog/students-guide-to-raft/#:~:text=accelerated%20log%20backtracking%20optimization), so tailoring it to the snapshot mechanism is very difficult. 

A workaround is to upgrade Binary Exponential: the initial value in `successiveLogConflict` is set to 3 instead of 0. Intuitively, this initial value represents the number of AppendEntries RPC round trip we can save when there is a lagging follower. However, setting the value to 3 is kind of intentional, because in `snapcommon()` of `test_test.go`, there are 11 `Start()` being called in each iteration, during which a server may be disconnected or crashed. 

But I think it is ok because the state machine will do snapshot quite often, and therefore whatever log backtracking mode we are using, we will inevitably send InstallSnapshot RPC request quickly after several failing AppendEntries RPC requests. Moreover, the test result turns out to be quite good.