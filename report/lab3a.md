# Lab 3A: Fault-tolerant Key/Value service without snapshots

http://nil.csail.mit.edu/6.824/2021/labs/lab-kvraft.html#:~:text=Part%20A:%20Key/value%20service%20without%20snapshots

## Design

### Code structure

The base code is misleading. There is no need to have both `GetArgs` and `PutAppendArgs`, which make it very hard to extract and reuse common code, both at client side and at server side. Just having a `KVArgs` with a field `Op` to distinguish between Get, Put, and Append is enough. So should `GetReply` and `PutAppendReply` be replaced by `KVReply`. Doing this will make life easier.

Another reason of doing so is that I don't know how to use design patterns in Go. I only know how to do that in Java, but Java's OOP support is significantly different from Go's. Therefore, it is hard for me to use interfaces like `IArgs` and `IReply` to write reuseable code. (In fact, I tried and it worked, but I soon realized this makes little sense.)

> `IArgs` is C#'s naming convention: the first `I` means interface. It is kind of funny to use this name when we are using Go and comparing it with Java.

### IDs

There are many types of IDs used in this system: 
- `cid` is the clerk ID; 
- `tid` is the trace ID, increasing monotonically for each new RPC request; 
- `xid` is the transaction ID, increasing monotonically for each GET, PUT, and APPEND operation. So, a `xid` can have many `tid` because of retires.

`tid` and `xid` are unique to one clerk, which means they may duplicate among clerks. So, `(cid, tid)` uniquely identifies a request and therefore its reply, while `(cid, xid)` uniquely identifies a transaction.

Although I considered `tid` as an option in lab 2, I make it a must at the beginning of development in this lab.

### Find leaders

### Server Architecture

What defines a good server architecture are the linearizability guarantee (i.e. correctness), the performance. Good code structure is also a criteria, but it comes naturally after the former two factors.

1️⃣ Linearizability Guarantee

The tester is guaranteed to use a client to do one operation at a time. It will call a function, wait for however long to get the result, and then call another function. A reason may be that it is hard to write a test to check the linearizability if a client is allowed to do multiple operations simultaneously.

But, anyway, it is really helpful to have this restriction, because we can focus on the server side. 

To achieve linearizability, we just handle the requests in a FIFO way. After a request comes in, we put it the argument of `Start`. Another goroutine is responsible for consuming the apply channel. It receives a command, which is a request, and perform the corresponding operation on the state machine, namely, a string-to-string map. Note that a single consumer naturally guarantees linearizability.

2️⃣ Performance

To achieve good performance, I drew inspiration from Redis and the `epoll()` function.

Redis is single-threaded for a number of reasons. Among them, what we can learn from for this lab is that single-thread is not intrinsically inferior to multi-thread. If the task is not CPU-bounded, then single-thread actually saves the cost of synchronizing multiple threads, coming from both using synchronization primitives and coding such program (the mental cost).

This K/V service is just like Redis, whose task can be efficiently handled using a hash table, a performant data structure to reduce CPU usage. So, we can just use single thread to handle the main logic, as the bottleneck for this service is not  CPU.

What can be the bottle neck of this service? Network I/O and memory, which are also the things that trouble Redis. Note that CPU and Disk I/O may be the bottleneck for the Raft library (Lab 2), but let's not consider them here.

We will not focus on memory size until lab 3B, because (1) the testcases doesn't try to fill up the map (maybe lab 4 will do so), so there's no need to worry about OOM; (2) memory size affects the snapshot speed and the snapshot size, but we don't need to consider snapshot in this lab.

So, the key problem is to handle Network traffic efficiently. Here, we can learn from how `epoll()` works. Below is my design:

- Each clerk corresponds to one client session, based on `cid`. The session contains a conditional variable `cond`, a `lastXid` denoting the last completed operation, a `lastResp` object storing the result of the last completed operation. 
  > Note that we are using "clerk" and "client" interchangeably. In fact, a "clerk" is the instance made from the client library (we write) of this K/V service, while a "client" is the actual client side using the K/V service, e.g., the tester. So, a client can have multiple clerks in theory, but such cases are rare. As there is one clerk per client in the tester, we can just call a clerk client.
- T1: After receiving a new request (`args`), get the client session based on its `cid`. If there's no such client session, create one. 
- T1: if `lastXid` is greater than `args.Xid`, suggesting the request is an old one, reply failure.
- T1: if `lastXid` equals to `args.Xid`, reply success with `lastResp`. Here, we are implementing duplicate detection as well as using the cache for fast response.
- T1: call `Start()` and wait on `cond` when `lastXid` is less than `args.Xid`.
- T2: As mentioned earlier, the request will be replicated on a majority of Raft instances and will be consumed through the apply channel. After consumption, `lastXid` is set to `args.Xid` (a greater value) and `lastResp` is set to a new response object. Now, we can broadcast on `cond`.
- T1: this goroutine wakes up if its operation is handled and therefore the handler is returned. The clerk will get the completed operation result and starts a new one in a while.

Note that T1 is the handler goroutine and T2 is the apply channel consumer. When there are lots of clerks doing requests, there will be many T1 waiting. But there will be only one T2.

A lot of goroutines waiting and waking up on condition is like I/O multiplexing. We design the system in this way is like implementing an event-drive architecture. Thanks to the fact that goroutines are so light-weighted compared with threads, our implementation can be a lot easier.

### Apply Log entries

I now use a conditional variable to implement the applying entry logic instead of using a ticker (see [lab 2B report](./lab2b.md#applylogentryticker)), which will save CPU from spinning around and will apply new entries faster. The code written by the professor in lecture 8 reminded me of this optimization, but I didn't do this until lab 3A because it really starts to require more performance. 

Another reason may be that I don't like to use such primitive, which seems deadlock-prone for me. But since we are already using it in lab 3A's code, let's use it anyway.

## Experiences

### Difficulty

Like lab 1, the difficulty largely comes from the system design. If we know about Redis, `epoll()`, session and etc., the difficulty will be reduced. But, as we are using the Raft library we built by ourselves, when a bug happens, we may also need to fix the Raft code, which adds extra complexity compared with lab 1.

### Snapshot

Lab 3A does not involve snapshot, which requires us to turn this feature off. However, I found my implementation in Lab 2D not completely correct. I did some fixing to pass the tests of this lab, but I want to leave the correct implementation to Lab 3B, which is a more realistic scenario of using snapshots, helping me understand it better.


### Optimization

Never do premature optimization, which, in most of the times, is the piece of code causing bugs. Although I know this rule, when I do system design, I tend to fall into this mindset of mixing achieving basic functionality and do optimization.