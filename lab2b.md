# Lab 2B: Raft log

## Design

- Add another schedule job: applying committed log entries for all servers.
- According to [this visualization tutorial](https://thesecretlivesofdata.com/raft/#replication), the leader does not send AppendEntries RPC request proactively to replicate new client commands, but only at heartbeat. In this way, the follower will commit the new log entries at the 2nd heartbeat after a new client command sent to the leader if everything goes well.

## Experiences
- After finishing the code of lab 2b, run tests of lab 2a first, which should be passed because only a little more code added to the election part.
- This lab is indeed hard in terms of difficulty compared with lab 2A, which is considered as moderate. The rise in difficulty mainly results from understanding log replication, both the idea (theory) and implementation details, which is significantly harder than understanding leader election. 
  
  As in the [Instructors’ Guide to Raft](https://thesquareplanet.com/blog/instructors-guide-to-raft/), Raft is easier to understand than Paxos not because its idea is simpler, but because of Figure 2, the ultimate guide offered by the author. 
  
  Leaving the comparison with Paxos aside, I agree with the idea especially when I don't understand the Receiver implementation #3 to #5 of AppendEntries RPC, which I consider the hardest of all. I did not grasp the theory of Raft perfectly, then when I don't understand Figure 2, what should I do? Luckily, [the last question in the Raft Q&A](https://thesquareplanet.com/blog/raft-qa/) where the questioner gave a detailed example is very helpful for me to gain better understanding and to work out a solution. 

  Another possible solution is to turn to LLM, but I deliberately not to do so for a few reasons: (a) I want to practice my abilities to read papers; (b) This lab was at 2021, before the first LLM product ChatGPT appeared. If people at that time could do it, I should also do it in the same way; (c) AI may be wrong.

