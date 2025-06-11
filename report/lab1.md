# Lab 1: MapReduce

http://nil.csail.mit.edu/6.824/2021/labs/lab-mr.html

## Coordinator

- The coordinator only responds to worker, never talking to worker proactively, just like a regular HTTP server. In other words, just pull, no push.
- The coordinator handles requests single-threadedly, like Redis, by using only one http handler and synchronize this method. Since it only dispatches orders, it won't be a bottleneck. In addition, single thread saves me from (potential) troubles dealing with concurrency. 
- The coordinator behaves like a finite state machine given the requests as inputs.

## Worker
- A worker periodically (every 1 second) pings to the coordinator, reporting its state and receiving orders from the coordinator.

## Experiences
- It's better to develop the communication mechanism and the state transition logic first rather than focusing on implementing the actual map/reduce tasks. Architecture matters!
- As in the GFS paper, "extensive and detailed diagnostic logging has helped immeasurably". The logs should "record many significant events and all RPC requests and replies". We should do this too.
- This lab is moderate (not hard) in terms of difficulty because:
  - From today's viewpoint (not 20 years ago), implementing MapReduce is essentially building a job scheduling platform. In addition, there are numerous similar implementations, such as Java's thread pool and Azure Pipeline. If you know these before, nothing will be too hard.
  - My backend development experience helps apply engineering practices to the lab, boosting both developing and debugging.
  - This lab does not require everything discussed in the MapReduce paper, leaving much freedom as well as reducing difficulty.


## Updates

> This section is written after this lab is done, i.e., not a part of the original report.

I guess the lab page marks this lab as "moderate/hard" because of the efforts to learn Go and get familiarity with it.