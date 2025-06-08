# Lab 2D: Raft log compaction

http://nil.csail.mit.edu/6.824/2021/labs/lab-raft.html#:~:text=Part%202D:%20log%20compaction


## Design

- Before lab 2D, we are used to using index of `log[]` as the log index. But as mentioned in Hint #3, we need to refactor our code to find another way to store and read log indices before starting to implement snapshot mechanism. I add a `Index` field to `LogEntry` to achieve this.

## Experiences

- Refactoring takes efforts, because Figure 2 is based on using index of `log[]` as the log index. We have nothing to follow but have to work out our own solution (cover all the corner cases).