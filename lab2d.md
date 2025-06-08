# Lab 2D: Raft log compaction

http://nil.csail.mit.edu/6.824/2021/labs/lab-raft.html#:~:text=Part%202D:%20log%20compaction


## Design

- Before lab 2D, we are used to using index of `log[]` as the log index. But as mentioned in Hint #3, we need to refactor our code to find another way to store and get log indices before starting to implement the snapshot mechanism. I add a `Index` field to `LogEntry` to achieve this.

## Experiences

- Refactoring takes efforts, because Figure 2 is based on using index of `log[]` as the log index. We have nothing to follow but have to work out our own solution (cover all the corner cases).
- This lab is particularly hard in terms of difficulty. Despite the fact that Figure 13 probably gives detailed instructions, nobody tells us how to combine Figure 2 and Figure 13, so we must figure out all the corner cases after adding the snapshot mechanism to the existing system. Let alone the code becomes nasty.