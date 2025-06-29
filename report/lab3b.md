# Lab 3B: Fault-tolerant Key/Value service with snapshots

http://nil.csail.mit.edu/6.824/2021/labs/lab-kvraft.html#:~:text=Part%20B:%20Key/value%20service%20with%20snapshots

## Design

- How to decide whether the Raft state size is approaching `maxraftstate`? use a conditional variable.
  - The server starts a goroutine that waits on this conditional variable if the Raft size is small. If the Raft size is big enough, take a snapshot.
  - Broadcast the conditional variable at the end of `persist()` so that the server gets informed.
- We can refer to the tester code of lab 2 on how to use these snapshot APIs. Our code basically does the same thing.
- Although `rf.persister.SaveStateAndSnapshot()` guarantees to "save both Raft state and K/V snapshot as a single atomic action", I consider it somehow unreal. In real settings, their sizes are both quite large, so anything can happen during saving them to disks. To guarantee the correctness of persisting both snapshot and the Raft state in the face of crashes, I persist the new snapshot first, and then persist the trimmed log. If the server crashes just after the new snapshot is persisted, it is ok because the apply channel consumer will ignore the entries whose indices are less than or equal to the snapshot's `LastIncludedIndex`.

## Experiences

- This lab is moderate/hard in terms of difficulty, which is definitely easier than lab 3A if we take it seriously. Much of the code we need to write has already appeared in previous labs, so we just need to adapt them. 
- Make `persist()` fine-grained: pass an argument to decide whether to persist snapshot along with the Raft state, because it takes time to copy objects.
- The gob encoding/decoding is error-prone. When a user-defined object is decoded from bytes, we should manually create a new object by copying the fields of the deserialized object instead of directly using it. This is a bug that stuck me for quite a long time. I guess something has to do with Go's memory model, which I know little about.
- We can disable log easily with `log.SetOutput(ioutil.Discard)`, which can boost performance. Note that since Go 1.16 (we are using 1.15), `ioutil.Discard` has been moved to `io.Discard`.