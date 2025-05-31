# Lab 2A: Raft leader election

http://nil.csail.mit.edu/6.824/2021/labs/lab-raft.html#:~:text=Part%202A:%20leader%20election

## Design
- Broadly speaking, this lab asks us to implement 2 RPC handlers and 2 schedule jobs.
  - AppendEntries RPC handler and RequestVote RPC handler
  - Leader heartbeat and follower election timeout
- Although I prefer to synchronize the whole function to avoid race condition, it is not possible to do so when we send RPC request and later handle the reply. They must be in another goroutine, otherwise the performance is unaccepted. So, there will be multiple lock() and unlock(). 

  An important thing to keep in mind is that RPC takes time and anything can happen during the process. Thus, we should constantly check the term in arguments, replies and the instances. We should also check the instances' states before and after the RPC.

## Experiences

- This lab is significantly harder than lab1 in terms of difficulty -- why the lab page marks it as "moderate"? It should be hard.
  - lab 2A requires us to build the underlying communicating mechanism for all the following labs, which is a serious thing to take. If we don't make it 100% correct, we will probably debug this lab when doing later ones (but I guess most people won't realize it is lab 2A that makes troubles, which is even worse). The idea is similar when doing later labs, as they will also be building blocks.
- Do extensive reading to make sure you fully understand Raft's mechanism before coding, which will pay off and save time.
- As in the [Students' Guide to Raft](https://thesquareplanet.com/blog/students-guide-to-raft/), Figure 2 in the paper is a must. Every detail must be followed, otherwise troubles occur.
- Reading the code in `test_test.go` is helpful to identify bugs.
- Deadlocks are painful to find and debug -- that's why I prefer lock-free programming. Here is a bug I underwent:
  - Normally, we use `defer` to remind ourselves to release the lock. This is indeed a benefit Go offers.
  - ```go
    rf.mu.Lock()
    defer rf.mu.Unlock()
    
    if args.Term < rf.currentTerm {
        reply.Term = rf.currentTerm
        reply.VoteGranted = false
        return // deferred statement will be called after return
    }
    ```
  - However, there are situations where we can not use `defer` because `defer` works in function level, not block level. I forgot to manually unlock the mutex, causing deadlock.
  - ```go
    for rf.killed() == false {
        rf.mu.Lock()
    
        if rf.state != STATE_FOLLOWER {
            rf.mu.Unlock() // unlock manually, otherwise causing deadlock
            continue
        }
    }
    ```
  - Although an IIFE (immediately invoked function expression, a concept in JS) can do the tick, it is not always possible. In addition, it creates an extra level of indentation, which is bad.
  - ```go
    for rf.killed() == false {
        func () { // an IIFE allows using `defer`
            rf.mu.Lock()
            defer rf.mu.Unlock()
            
            if rf.state != STATE_FOLLOWER {
                return
            }
        }()
    }
    ```

