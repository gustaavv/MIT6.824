package mr

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
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

// for sorting by key.
type ByKey []KeyValue

// for sorting by key.
func (a ByKey) Len() int           { return len(a) }
func (a ByKey) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByKey) Less(i, j int) bool { return a[i].Key < a[j].Key }

//
// use ihash(key) % NReduce to choose the reduce
// task number for each KeyValue emitted by Map.
//
func ihash(key string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() & 0x7fffffff)
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

type WorkerInstance struct {
	id            int
	state         string
	mu            sync.Mutex
	mapNumber     int
	reduceNumber  int
	exitCountdown int
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
	wi.exitCountdown = WORKER_EXIT_COUNTDOWN

	log.Printf("Start worker: %d", wi.id)
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		ticker := time.NewTicker(WORKER_PING_FREQUENCY)
		defer ticker.Stop()

		for {
			<-ticker.C

			args := new(WorkerPingArgs)
			reply := new(WorkerPingReply)

			wi.mu.Lock()
			args.State = wi.state
			args.MapNumber = wi.mapNumber
			args.ReduceNumber = wi.reduceNumber
			wi.mu.Unlock()
			args.WorkerId = wi.id
			args.TraceId = generateTraceId()

			log.Printf("worker ping args = %s", args)
			callSuccess := call("Coordinator.HandleWorkerPing", args, reply)
			log.Printf("worker ping reply = %s", reply)

			wi.mu.Lock()

			if callSuccess {
				wi.exitCountdown = WORKER_EXIT_COUNTDOWN
			} else {
				wi.exitCountdown--
				log.Printf("Ping coordinator fails. Exit countdown: %d", wi.exitCountdown)
			}

			if wi.exitCountdown == 0 {
				wg.Done()
				return
			}

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

			switch reply.Order {
			case "map":
				go doMap(reply, mapf)
			case "reduce":
				go doReduce(reply, reducef)
			case "exit":
				log.Printf("Exit order from coordinator")
				wg.Done()
			}
		}
	}()

	wg.Wait()
	log.Printf("Worker %v exit", wi.id)
}

func doMap(reply *WorkerPingReply, mapf func(string, string) []KeyValue) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		log.Printf("Map task %v takes %.3f seconds", reply.MapNumber, elapsed.Seconds())
	}()
	log.Printf("Start map task %d, map file = %s", reply.MapNumber, reply.MapFile)

	filename := reply.MapFile
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("cannot open %v", filename)
	}
	content, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("cannot read %v", filename)
	}
	_ = file.Close()
	log.Printf("Finish reading map file content %s", filename)
	kva := mapf(filename, string(content))

	sort.Sort(ByKey(kva))

	mapping := make([][]KeyValue, reply.R)

	for _, kv := range kva {
		h := ihash(kv.Key) % len(mapping)
		mapping[h] = append(mapping[h], kv)
	}
	wg2 := new(sync.WaitGroup)
	wg2.Add(len(mapping))
	for i, kvList := range mapping {
		i := i
		kvList := kvList
		go func() {
			intermediateFileName := fmt.Sprintf("mr-%v-%v", reply.MapNumber, i)
			if fileExists(intermediateFileName) {
				log.Printf("File %v already exists", intermediateFileName)
				wg2.Done()
				return
			}
			tempFile, err := ioutil.TempFile(TEMP_FOLDER, intermediateFileName+"-*")
			if err != nil {
				log.Fatal(err)
			} else {
				log.Printf("Created temporary file %v", tempFile.Name())
			}
			defer os.Remove(tempFile.Name())

			encoder := json.NewEncoder(tempFile)
			err = encoder.Encode(kvList)
			if err != nil {
				log.Fatal(err)
			}
			_ = tempFile.Close()

			_ = os.Rename(tempFile.Name(), intermediateFileName)
			log.Printf("Renamed temporary file %v to %v", tempFile.Name(), intermediateFileName)
			wg2.Done()
		}()

	}
	wg2.Wait()

	log.Printf("Finish map task %d", reply.MapNumber)
	wi.mu.Lock()
	defer wi.mu.Unlock()
	wi.state = "map-done"
}

func doReduce(reply *WorkerPingReply, reducef func(string, []string) string) {
	start := time.Now()
	defer func() {
		elapsed := time.Since(start)
		log.Printf("Reduce task %v takes %.3f seconds", reply.ReduceNumber, elapsed.Seconds())
	}()
	log.Printf("Start reduce task %d", reply.ReduceNumber)

	outFilename := fmt.Sprintf("mr-out-%v", reply.ReduceNumber)
	if fileExists(outFilename) {
		log.Printf("File %v already exists", outFilename)
		return
	}

	files, _ := filepath.Glob(fmt.Sprintf("mr-*-%v", reply.ReduceNumber))
	if len(files) != reply.M {
		log.Fatalf("reduce task expects %v files but got %v", reply.M, len(files))
	}
	log.Printf("Start reading intermediate files %v", files)

	jsonObjList := make([][]KeyValue, len(files))
	wg := new(sync.WaitGroup)
	wg.Add(len(files))
	for i, f := range files {
		i := i
		f := f
		go func() {
			file, err := os.Open(f)
			if err != nil {
				log.Fatal(err)
			}
			dec := json.NewDecoder(file)
			var jsonObj []KeyValue
			if err := dec.Decode(&jsonObj); err != nil {
				log.Fatal(err)
			}
			_ = file.Close()
			jsonObjList[i] = jsonObj
			wg.Done()
		}()
	}
	wg.Wait()

	kva := []KeyValue{}
	for _, jsonObj := range jsonObjList {
		kva = append(kva, jsonObj...)
	}

	log.Printf("Finish reading intermediate files %v", files)
	sort.Sort(ByKey(kva)) // should we really sort these kv pairs here?

	tempFile, err := ioutil.TempFile(TEMP_FOLDER, outFilename+"-*")
	if err != nil {
		log.Fatal(err)
	} else {
		log.Printf("Created temporary file %v", tempFile.Name())
	}
	defer os.Remove(tempFile.Name())

	i := 0
	for i < len(kva) {
		j := i + 1
		for j < len(kva) && kva[j].Key == kva[i].Key {
			j++
		}
		values := []string{}
		for k := i; k < j; k++ {
			values = append(values, kva[k].Value)
		}
		output := reducef(kva[i].Key, values)

		// this is the correct format for each line of Reduce output.
		_, _ = fmt.Fprintf(tempFile, "%v %v\n", kva[i].Key, output)

		i = j
	}

	_ = tempFile.Close()
	_ = os.Rename(tempFile.Name(), outFilename)
	log.Printf("Renamed temporary file %v to %v", tempFile.Name(), outFilename)
	log.Printf("Finish reduce task %d", reply.ReduceNumber)
	wi.mu.Lock()
	defer wi.mu.Unlock()
	wi.state = "reduce-done"
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
		log.Print(err)
		return false
	}
	defer c.Close()

	err = c.Call(rpcname, args, reply)
	if err == nil {
		return true
	}

	log.Print(err)
	return false
}
