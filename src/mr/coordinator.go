package mr

import (
	"log"
	"sync"
	"time"
)
import "net"
import "os"
import "net/rpc"
import "net/http"

//type WorkerStatus struct {
//	id         int
//	state      string
//	lastTalkAt time.Time
//}

type MapTask struct {
	id             int
	file           string
	assigned       bool
	done           bool
	lastAssignedAt time.Time
}

type ReduceTask struct {
	id             int
	assigned       bool
	done           bool
	lastAssignedAt time.Time
}

type Coordinator struct {
	// Your definitions here.

	// these 2 fields are just for Done() function
	done      bool
	doneMutex sync.Mutex

	mu sync.Mutex // global mutex for all the state mutations below

	finWait   bool
	finWaitAt time.Time
	M         int // map partition constant
	R         int // reduce partition constant
	//workers map[int]*WorkerStatus
	numMap      int
	numReduce   int
	MapTasks    []*MapTask
	ReduceTasks []*ReduceTask
}

func (c *Coordinator) getNewMapTask() *MapTask {
	for _, task := range c.MapTasks {
		if !task.done && !task.assigned {
			return task
		}
	}
	return nil
}

func (c *Coordinator) getNewReduceTask() *ReduceTask {
	for _, task := range c.ReduceTasks {
		if !task.done && !task.assigned {
			return task
		}
	}
	return nil
}

func (c *Coordinator) timeoutAssignedTasks() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finWait {
		return
	}

	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		log.Printf("Checking timeout assigned tasks takes %.3f seconds", elapsed.Seconds())
	}()

	now := time.Now()

	if c.numMap > 0 {
		for _, task := range c.MapTasks {
			if task.assigned && !task.done && now.Sub(task.lastAssignedAt) > ASSIGNED_TASKS_TIMEOUT {
				task.assigned = false
				log.Printf("Map task %v timeout, revoke assignment", task.id)
			}
		}
	}

	if c.numMap == 0 && c.numReduce > 0 {
		for _, task := range c.ReduceTasks {
			if task.assigned && !task.done && now.Sub(task.lastAssignedAt) > ASSIGNED_TASKS_TIMEOUT {
				task.assigned = false
				log.Printf("Reduce task %v timeout, revoke assignment", task.id)
			}
		}
	}
}

// Your code here -- RPC handlers for the worker to call.

func (c *Coordinator) HandleWorkerPing(args *WorkerPingArgs, reply *WorkerPingReply) error {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		log.Printf("Handle request (trace id = %v) takes %.3f seconds", args.TraceId, elapsed.Seconds())
	}()
	// just make coordinator single-threaded (like Redis), since coordinator won't be a bottleneck
	c.mu.Lock()
	defer c.mu.Unlock()

	log.Printf("worker ping args = %s", args)
	reply.R = c.R
	reply.M = c.M
	reply.TraceId = args.TraceId

	switch args.State {
	case "idle":
		if c.numMap > 0 {
			task := c.getNewMapTask()
			if task != nil {
				reply.Order = "map"
				task.assigned = true
				task.lastAssignedAt = time.Now()
				reply.MapFile = task.file
				reply.MapNumber = task.id
				log.Printf("Assign map task %d to worker %d", task.id, args.WorkerId)
			} else {
				reply.Order = "pong"
			}

		} else if c.numReduce > 0 {
			task := c.getNewReduceTask()
			if task != nil {
				reply.Order = "reduce"
				task.assigned = true
				task.lastAssignedAt = time.Now()
				reply.ReduceNumber = task.id
				log.Printf("Assign reduce task %d to worker %d", task.id, args.WorkerId)
			} else {
				reply.Order = "pong"
			}
		} else if c.finWait {
			reply.Order = "exit"
		} else {
			reply.Order = "pong"
		}

	case "map":
		reply.Order = "pong"

	case "map-done":
		task := c.MapTasks[args.MapNumber]
		if !task.done {
			task.done = true
			c.numMap--
			log.Printf("map task %d finished by worker %d. %d map tasks left", task.id, args.WorkerId, c.numMap)
		}
		reply.Order = "idle"

		if c.numMap == 0 {
			log.Printf("All map tasks finished, entering reduce stage")
		}

	case "reduce":
		reply.Order = "pong"

	case "reduce-done":
		task := c.ReduceTasks[args.ReduceNumber]
		if !task.done {
			task.done = true
			c.numReduce--
			log.Printf("reduce task %d finished by worker %d. %d reduce tasks left", task.id, args.WorkerId, c.numReduce)
		}
		reply.Order = "idle"

		if c.numReduce == 0 {
			c.finWait = true
			c.finWaitAt = time.Now()
			log.Printf("All reduce tasks finished, entering fin wait stage")
			go func() {
				log.Printf("Coordinator will shutdown in %v seconds", FIN_WAIT_DURATION)
				afterChan := time.After(FIN_WAIT_DURATION)
				<-afterChan

				c.doneMutex.Lock()
				defer c.doneMutex.Unlock()
				c.done = true
				log.Printf("Coordinator shutdown")
			}()
		}
	}
	log.Printf("worker ping reply = %s", reply)
	return nil
}

//
// start a thread that listens for RPCs from worker.go
//
func (c *Coordinator) server() {
	_ = rpc.Register(c)
	rpc.HandleHTTP()
	//l, e := net.Listen("tcp", ":1234")
	sockname := coordinatorSock()
	_ = os.Remove(sockname)
	l, e := net.Listen("unix", sockname)
	if e != nil {
		log.Fatal("listen error:", e)
	}
	go http.Serve(l, nil)
}

//
// main/mrcoordinator.go calls Done() periodically to find out
// if the entire job has finished.
//
func (c *Coordinator) Done() bool {
	c.doneMutex.Lock()
	defer c.doneMutex.Unlock()
	return c.done
}

//
// create a Coordinator.
// main/mrcoordinator.go calls this function.
// nReduce is the number of reduce tasks to use.
//
func MakeCoordinator(files []string, nReduce int) *Coordinator {
	configLog()

	if _, err := os.Stat(TEMP_FOLDER); os.IsNotExist(err) {
		err := os.Mkdir(TEMP_FOLDER, os.ModePerm)
		if err != nil {
			log.Fatalf("create temp folder failed: %v", err)
		} else {
			log.Printf("Create temp folder: %s", TEMP_FOLDER)
		}
	}

	log.Printf("Make coordinator with args: %d files = %v, nReduce = %v", len(files), files, nReduce)

	c := Coordinator{}
	c.M = len(files)
	c.R = nReduce
	c.numMap = c.M
	c.numReduce = c.R
	c.MapTasks = make([]*MapTask, c.numMap)
	c.ReduceTasks = make([]*ReduceTask, c.R)

	for i, file := range files {
		c.MapTasks[i] = &MapTask{i, file, false, false, time.Now()}
	}

	for i := 0; i < c.R; i++ {
		c.ReduceTasks[i] = &ReduceTask{i, false, false, time.Now()}
	}

	c.server()
	log.Printf("Coordinator starts, entering map stage")

	go func() {
		// periodically reset assigned but un-finished tasks to un-assigned
		ticker := time.NewTicker(SERVER_CKECK_TIMEOUT_ASSIGNED_TASKS_FREQUENCY)
		defer ticker.Stop()
		for {
			<-ticker.C
			c.timeoutAssignedTasks()
		}
	}()

	return &c
}
