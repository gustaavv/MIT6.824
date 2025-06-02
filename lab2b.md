# Lab 2B: Raft log

## Design

- Add another schedule job: applying committed log entries for all servers.
- The leader does not send AE RPC request proactively to replicate new client's commands, only at heartbeat timeout. In this way, the follower will commit the new log entries in the 2nd round of the heartbeat timeout after a new client command sent to the leader.

## Experiences
- After finishing the code of lab 2b, run tests of lab 2a first, which should be passed because only a little more code added to the election part.
- As in the [Instructors’ Guide to Raft](https://thesquareplanet.com/blog/instructors-guide-to-raft/), Raft is easier to understand than Paxos not because its idea is simpler, but because of Figure 2, the ultimate guide offered by the author. Leaving the comparison with Paxos aside, I agree with the idea especially when I don't understand the Receiver implementation #3 to #5 of AppendEntries RPC. I did not grasp everything about Raft, then when I don't understand Figure 2, what should I do? Luckily, [the last question in the Raft Q&A](https://thesquareplanet.com/blog/raft-qa/) where the questioner gave a detailed example is very helpful for me to gain better understanding. 