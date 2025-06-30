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
- The gob encoding/decoding is error-prone. When a user-defined object is decoded from bytes, we should manually create a new object and copy the fields of the deserialized object instead of directly using it. This is a bug that stuck me for quite a long time. I guess something has to do with Go's memory model, which I know little about.
- We can disable log easily with `log.SetOutput(ioutil.Discard)`, which can boost performance. Note that since Go 1.16 (we are using 1.15), `ioutil.Discard` has been moved to `io.Discard`.
- To detect goroutine leak, I use a ticker to periodically call `runtime.NumGoroutine()` and write it to a file. After the tester finishes, we can calculate the maximum goroutine number during execution.
  
  Goroutine leak happens only on GitHub Actions: Normally, the number never exceeds 2,000, but sometimes `race: limit on 8128 simultaneously alive goroutines is exceeded, dying` error appears. I consider it as a glitch of GitHub Actions' machines, but it can be controlled by a configuration parameter, the request timeout of the clerk: if a clerk does not get a succeeded response in this period of time, it will resend requests. This process will go forever until the clerks get the response. Here is a table about how the value of request timeout affects goroutine leak frequency on GitHub Actions:
  
  | Request timeout | Goroutine leak frequency |
  | --------------- | ------------------------ |
  | 1s              | No                       |
  | 500ms           | Seldom                   |
  | 300ms           | Often                    |

  Goroutine number increases because the RPC call has to be in a new goroutine in order to implement the timeout feature. So, it may lead to many new goroutines to complete a single transaction. But I am not sure why the glitch happens, maybe it is my code rather than GitHub Actions that has problems. Maybe the resources of GitHub Actions' machines are limited.

- The lab page says that "A reasonable amount of time to take for the Lab 3 tests is 400 seconds of real time", but it runs lab 3A for 290s and lab 3B for 160s, which means its time to run lab 3 tests is definitely greater than 450s. This is a conflict, and I doubt whether the benchmark is achievable.
  
  In terms of my experience to optimize the time performance, the request timeout plays a pivotal role among all factors.
  
  | Request timeout | Time (My machine) | Time (GitHub Actions' machine) |
  | --------------- | ----------------- | ------------------------------ |
  | 1s              | 480s              | 495s                           |
  | 500ms           | 430s              | 459s                           |
  | 300ms           | 415s              | 440s                           |

  But, as talked above, request timeout of 300ms often causes goroutine leak, which is unacceptable. So, I am not intended to do any further optimization.
