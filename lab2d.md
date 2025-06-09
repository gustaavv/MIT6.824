# Lab 2D: Raft log compaction

http://nil.csail.mit.edu/6.824/2021/labs/lab-raft.html#:~:text=Part%202D:%20log%20compaction


## Design

- Before lab 2D, we are used to using index of `log[]` as the log index. But as mentioned in Hint #3, we need to refactor our code to find another way to store and get log indices before starting to implement the snapshot mechanism. I add a `Index` field to `LogEntry` to achieve this.
- The snapshot mechanism requires a new RPC handler.
- When the leader wants to send an AppendEntries RPC request, if `nextIndex` is no greater than the snapshot's `LastIncludedIndex`, which means the follower is lagging behind, an InstallSnapshot RPC request will be sent instead.

## Experiences

- Refactoring takes efforts, because Figure 2 is based on using index of `log[]` as the log index. We have nothing to follow but have to work out our own solution (cover all the corner cases).
- This lab is particularly hard in terms of difficulty. Despite the fact that Figure 13 probably gives detailed instructions, nobody tells us how to combine Figure 2 and Figure 13, so we must figure out all the corner cases after adding the snapshot mechanism to the existing system. Let alone the code becomes nasty (hard to understand) as a result of covering those corner cases.
- When stuck with a bug for a long time, the built-in log package gets unhandy, because it does not support log level and the log we adds to find that bug belongs to DEBUG level. After successful debugging, those log should be turned off (commented out). But I am also not really interested in installing a production-ready log framework, which also takes efforts to config.
- In terms of [log backtracking mode](./lab2b.md#Log%20backtracking), Conflict Term Bypassing is the best in terms of time performance, but it needs additional refactoring after implementing the snapshot mechanism. So, I switched to Binary Exponential in order to get the snapshot mechanism right. Although this mode will sometimes timeout (a 2 min limit on every 2D test), it is important to get things right first and after that do we consider optimization. This reflects separation of concern in coding.