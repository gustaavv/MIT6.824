# Lab 2C: Raft state persistence

http://nil.csail.mit.edu/6.824/2021/labs/lab-raft.html#:~:text=Part%202C:%20persistence

## Design

This lab requires only a little more code, so nothing particularly needs to be designed.

## Experiences

- Figure 2 says that persistent state should be "updated on stable storage before responding to RPCs". This is easy to do: simply put a `defer rf.persist()` at the beginning of each RPC handler. But there are other places where we need to call this function besides RPC handlers. How to spot those places?
  - I found it helpful to use IDE's "Find Usages" functionality to find all statements that involve the persistent state. Furthermore, GoLand can differentiate between "Value Read" and "Value Write", which saves more time because there are more reads than writes in most cases.
  - A slightly inferior approach is to do a global search of the field name, but there will be local variables with the same name.
  - Do not read through every line of the existing code to find where to add that function. It is both inefficient and error-prone.
- Go's `sync.Mutex` is not a reentrant lock. When a caller is already locked, the callee should not try to lock again. Otherwise, deadlock occurs. So we need to set a rule on which function acquires a certain lock. I often forget this because I am used to Java's reentrant locks.
- It seems that the tests doesn't simulate a real network environment where packets can lose, duplicate and reorder (I hope the tests cover all the 3 cases, not only the first one) until this lab. But nothing needs to be worried about, since we are strictly following Figure 2. Therefore, this lab will be easy in terms of difficulty. 