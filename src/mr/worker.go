package mr

import (
	"fmt"
	"os"
	"sync"
	"time"
)
import "log"
import "net/rpc"
import "hash/fnv"

//
// Map functions return a slice of KeyValue.
//
type KeyValue struct {
	Key   string
	Value string
}

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

type WorkerInstance struct {
	id           int
	state        string
	mu           sync.Mutex
	mapNumber    int
	reduceNumber int
}

var wi = new(WorkerInstance)

//
// main/mrworker.go calls this function.
//
func Worker(mapf func(string, string) []KeyValue,
	reducef func(string, []string) string) {
	configLog()

	wi.id = os.Getpid()
	wi.state = "idle"

	log.Printf("Start worker: %s", wi)
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			<-ticker.C

			args := new(WorkerPingArgs)
			wi.mu.Lock()
			args.State = wi.state
			args.MapNumber = wi.mapNumber
			args.ReduceNumber = wi.reduceNumber
			wi.mu.Unlock()
			args.WorkerId = wi.id
			args.TraceId = generateTraceId()

			reply := new(WorkerPingReply)
			log.Printf("worker ping args = %s", args)
			call("Coordinator.HandleWorkerPing", args, reply)
			log.Printf("worker ping reply = %s", reply)

			wi.mu.Lock()
			if reply.Order == "map" {
				wi.mapNumber = reply.MapNumber
			}
			if reply.Order == "reduce" {
				wi.reduceNumber = reply.ReduceNumber
			}
			if reply.Order != "pong" {
				wi.state = reply.Order
			}
			wi.mu.Unlock()

			// TODO
			switch reply.Order {
			case "map":
				go func() {
					log.Printf("Start map task %d, map file = %s", reply.MapNumber, reply.MapFile)
					time.Sleep(time.Second * 2)
					log.Printf("Finish map task %d", reply.MapNumber)
					wi.mu.Lock()
					defer wi.mu.Unlock()
					wi.state = "map-done"
				}()
			case "reduce":
				go func() {
					log.Printf("Start reduce task %d", reply.ReduceNumber)
					time.Sleep(time.Second * 2)
					log.Printf("Finish reduce task %d", reply.ReduceNumber)
					wi.mu.Lock()
					defer wi.mu.Unlock()
					wi.state = "reduce-done"
				}()
			case "exit":
				log.Printf("Exit order from coordinator")
				wg.Done()
			}
		}
	}()

	wg.Wait()
	log.Printf("Worker %v exit", wi.id)
}

//
// send an RPC request to the coordinator, wait for the response.
// usually returns true.
// returns false if something goes wrong.
//
func call(rpcname string, args interface{}, reply interface{}) bool {
	// c, err := rpc.DialHTTP("tcp", "127.0.0.1"+":1234")
	sockname := coordinatorSock()
	c, err := rpc.DialHTTP("unix", sockname)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	fmt.Println(err)
	return false
}
