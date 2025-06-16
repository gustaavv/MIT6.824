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

What defines a good server architecture are the linearizability guarantee (i.e. correctness), the performance and good code structure.

1️⃣ Linearizability Guarantee

2️⃣ Performance

To achieve good performance, I drew inspiration from Redis and the `epoll()` function.



3️⃣ Good Code Structure

## Experiences

### Snapshot

Lab 3A does not involve snapshot, which requires us to turn this feature off. However, I found my implementation in Lab 2D not completely correct. I did some fixing to pass the tests of this lab, but I want to leave the correct implementation to Lab 3B, which is a more realistic scenario of using snapshots, helping me understand it better.