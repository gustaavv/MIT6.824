# Lab 1: MapReduce

## Coordinator

- The coordinator only responds to worker, never talking to worker proactively, just like a regular HTTP server.
- The coordinator handles requests single-threadly, like Redis, by using only one http handler and synchronize this method. Since it only dispatches orders, it won't be a bottleneck. In addition, single thread saves me from troubles dealing with concurrency. 
- The coordinator behaves like a finite state machine given the requests as inputs

## Worker
- A worker periodically (every 1 second) pings to the coordinator, reporting its state and receiving orders from the coordinator.

## Experience
- It's better to develop the communication mechanism and the state transition logic first rather than focusing on implementing the actual map/reduce task. Architecture matters! 