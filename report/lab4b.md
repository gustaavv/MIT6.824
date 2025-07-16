# Lab 4B: Sharded Key/Value Server

http://nil.csail.mit.edu/6.824/2021/labs/lab-shard.html#:~:text=Part%20B:%20Sharded%20Key/Value%20Server


## Design

### My Design thinking
I will talk about how I came up with the reconfiguration protocol first, namely my thinking process, which I believe is what an open-source project should include.

1️⃣There should be consistency between reconfigurations and client operations. So, everything should go through Raft, and the log must be replayable under any circumstance.

2️⃣A reconfiguration involves multiple groups of servers, which requires careful coordination. We just need to coordinate among group leaders, the coordination inside a group is already done by Raft.

3️⃣During a reconfiguration, a server may send shards to other servers as well receive shards from other servers. We define the former one as *outShard*, and the latter one as *inShard*. If a server deletes its outShard data before other servers get them, the shard will be completely lost. So, a server is allowed to delete its outShard data only when all the servers need them get them. If servers only fetch their inShard data without sending outShard data proactively, there are clearly two phases for a server to perform a reconfiguration. The first phase is to get all the inShard data. After all other servers get their inShard data (the server's outShard data), it enters the second phase, where it can delete its outShard data. This reminds me of two phase commit. The atomicity provided by 2PC is good for coordination.

4️⃣During a reconfiguration, if servers continue serving client operations, consistency will break. Which server is going to handle requests to a certain shard? The one originally owns it, or the one going to receive it? What's worse, two consecutive fetches of an inShard data from a server may return different data. So, we'd better stop serving client operations during a reconfiguration.

### The reconfiguration protocol

Let's first define the server's state:

- `Config`: the server's current configuration.
- `(ReConfigNum, ReConfigStatus)`: a tuple indicating what stage the server is at in terms of reconfiguration. 
  - `ReConfigNum` is easy to understand, which is normally the number of the new configuration the server wants to switch to. 
  - `ReConfigStatus` has 3 possible values: `START`, `PREPARE` and `COMMIT`, which are different phases the server is going through during a reconfiguration.
  - This tuple changes as follows: `(0, COMMIT)`, `(1, START)`, `(1, PREPARE)`, `(1, COMMIT)`, `(2, START)`, ...
  - Note that this tuple is comparable.

Let T1 be the ticker goroutine that periodically queries the latest configuration and handle the reconfiguration process, and T2 be the apply message consumer goroutine. The reconfiguration protocol is as follows:

1️⃣ T1 finds a new configuration, whose number is `nextNum`, which is equal to `Config.Num + 1`. Note that the server is currently at `(Config.Num, COMMIT)`.

- Reconfigurations are preformed one by one instead of going to the latest one directly, which is easier.

2️⃣ T1 waits for all other servers to be at least `(Config.Num, COMMIT)`, which means all other servers are in the current configuration.

- What do we mean by "all servers"? They are the union of both groups in the current configuration and the new configuration.

3️⃣ T1 starts a `START` log entry and waits for it to be consumed.

T2 consumes the `START` entry: sets the reconfiguration tuple to be `(nextNum, START)`, and rejects any client operations that comes later.

4️⃣ T1 fetches all inShard data, then it starts a `PREPARE` log entry and waits for it to be consumed.

- By comparing the current configuration and the new configuration, we can know what inShard and outShard are.
- A server is only allowed to return data if it is at least at `(nextNum, START)`. So T1 will wait for other servers to be at least `(NextNum, START)`.

T2 consumes the `PREPARE` entry: adds the inShard data to the KV Store, and sets the reconfiguration tuple to be `(nextNum, PREPARE)`

5️⃣ T1 waits for all other servers to be at least `(NextNum, PREPARE)`, which means all other servers got their inShard data.

6️⃣ T1 starts a `COMMIT` log entry and waits for it to be consumed.

T2 consumes the `COMMIT` entry: deletes the oldShard data from the KV Store, sets `Config` to the new configuration, sets the reconfiguration tuple to be `(nextNum, COMMIT)`, and starts to handle client operations.

---

Although there are `PREPARE` and `COMMIT` phases, the protocol is not really a distributed transaction. From a single group's viewpoint, the reconfiguration process resembles transitions between different states (`START` -> `PREPARE` -> `COMMIT`). But strictly speaking, it is not a state machine, because it involves interacting with other groups (the outside of the world).

### Configuration consistency

Both clients and servers will periodically query the shard controller to get the latest configuration. At first sight, it may looks likes a cache-aside setting, where the shard controller stands aside from clients and servers, which may introduce inconsistency among the views of configuration between clients and servers.

We can guarantee consistency by add the two following rules:

- A request contains the latest configuration number the clerk knows. A server will reject the request if the number is different from its `Config.Num`.
- A server will reject a key that does not belong to its shards.

Generally speaking, it is ok for clients and servers to interact normally even if their views of the configuration are stale. But some test cases in lab 4b requires more than that, which asks the servers to respond to configuration changes fast enough. As the note says, querying configuration every 100ms is enough to pass those tests, which is not a big deal when coding.

### Duplicate detection across shard movement

The lab page hints that "You'll need to provide at-most-once semantics (duplicate detection) for client requests across shard movement."

Consider the following scenario to figure out why it is necessary:
- A client sends a request to a group. The server returns a succeeded reply, but it is lost in the network.
- A reconfiguration happens.
- The client thinks its request failed, thus resending the request to the new group. The new group consumes it again, causing wrong results.

Recall that I use session storing `lastXid` and `lastReply` for every clerk to implement duplicate detection in lab 3. For a given shard, the original group knows all the `lastXid`s involving this shard. However, a new group knows nothing.

My solution is simple. When the new group fetches inShard data, the original group returns the data as well as its session. The new group will update its session based on that of the original group. In this way, after the reconfiguration, the new group knows everything the original group knows, which achieves duplicate detection across shard movement.

But I do think this solution is naive and inefficient.

### Lots of clerks

Recall that a clerk is used to communicate to a group of servers atop Raft in lab 3. Now, in lab 4, we have multiple groups, so a sharded K/V clerk (or SKVClerk for short) will contain multiple clerks (let's call them group clerks, or GClerks for short), plus one shard controller clerk (or SCClerk for short).

For every new configuration, SKVClerk will create new GClerks as well delete existing GClerks to match the changes in `Config.Groups`. Recall that we are using `cid` for duplicate detection, so all GClerks of a SKVClerk should have the same `cid`. For convenience, set the ids of SKVClerk and its SCClerk to the same `cid` as well.

### Consistency when doing fast RPC

Generally speaking, for servers atop Raft, every RPC request received will go through the Raft network, being replicated, committed, and finally consumed by the server. While this guarantees consistency, it may be slow. 

Sometimes, we may want the server to respond quickly. For example, we may want to check whether the server is a Raft leader. In my implementation, the state of a Raft instance is stored in a field, indicating whether the instance is a leader, a follower, or a candidate. So, the server can just acquire the mutex and then return that state, which is pretty simple and fast.

However, when we want the server to return state data directly instead of going through Raft, inconsistency may happen. Let's consider the reconfiguration protocol. In step 4️⃣: "A server is only allowed to return data if it is at least at `(nextNum, START)`." If no such condition, the returned shard data may change in two consecutive fetches because the server hasn't entered `START` phase (the log entry is yet to be applied).

But with this condition, everything goes fine. What's more, now the followers of a group can serve such requests, reducing the load on the leader. The condition guarantees consistency, while the fast RPC handler boosts performance. It reminds me of Zookeeper, where reads are handled by followers, but user can call `sync()` to guarantee linearizability.

## Experiences

- This lab is pretty hard in terms of difficulty, mainly resulting from the need to work out a reconfiguration protocol.
- I have refactored a lot during debugging this lab.
  - After discussing it a lot in previous lab reports, I finally implemented a feature flag to enable fine-grained control about what components are allowed to log. For example, we can turn off the shard controller clerk's log while turning on the sharded K/V clerk's log. The code is tedious: add a boolean variable to the config, and check it before every logging statements.
  - The previous config only consists constant variables, which is impossible to apply different configs with the same code to different components. Now I define a struct containing all these variables, and give instances of the config struct to each component, so that they can use different config at their will.