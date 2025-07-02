# Lab 4A: Shard controller

http://nil.csail.mit.edu/6.824/2021/labs/lab-shard.html#:~:text=Part%20A:%20The%20Shard%20controller

## Design

- Since the K/V server in lab 3, the shard controller in lab 4a, and the sharded K/V server in lab 4b are all built atop Raft, we should refactor common code into a separate package to increase code reusability. In my code, the `atopraft` package contains base code for service built atop Raft, including the clerk and the server.
- The basic idea of the shard reassignment algorithm is to take shards from groups with the most shards and give them to the ones with the least shards. I just take shards one by one so that the algorithm can be relatively simple (though it is already complex enough). The worst time complexity can be bad, but we can set `NShards` to a small value rather than something like the number of hash slots in Redis, which is 16384.


## Experiences

- This lab is hard in terms of difficulty, not easy as said in the lab page. 
  
  The refactoring process greatly increases the difficulty, because in order to abstract common code, we need to use the OOP principle and some design patterns. But, I have not systematically learned Go (I only did the "a tour to Go" tutorial). Therefore, even with the help of LLM, I found it difficult to handle Go interfaces, value types and pointer types, which are used a lot to write reusable code.

  The shard reassignment algorithm is at least medium level in LeetCode's standard, not an easy one. In addition, using Go to solve algorithm problems is not as handy as using C++ to do the same thing.

- Although the refactoring process took efforts, later work will benefit from it for sure.
- In terms of the way to apply requests to the state machine, I am influenced by Redux ToolKit: Use `switch args.Op` to distinguish operation types first and then write state transition logic in corresponding `case` blocks. Maybe it is just a standard way to implement a state machine. Take a look at the `businessLogic()` function in both `scServer.go` and `kvServer.go`.
- After so many labs, I finally started to use (as well as realized the value of) cli tools to parse the log, e.g. grep, sed, and awk. Of course, most of the commands were written by LLM. I think these tools can serve as the solution to the problem I raised in [lab 2d](./lab2d.md#log-used-in-debug) that I want "a log with feature flags when debugging". But they may only work in this simple setting, namely to get the inputs and outputs of an algorithm, but not debug a distributed program.