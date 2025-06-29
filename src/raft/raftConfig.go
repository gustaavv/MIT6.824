package raft

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"runtime"
	"sync"
	"time"
)

/////////////////////////// shared parameters /////////////////////////////////

const STATE_FOLLOWER = "follower"
const STATE_LEADER = "leader"
const STATE_CANDIDATE = "candidate"

const TICKER_FREQUENCY = time.Millisecond * 1

const SUCCESSIVE_CONFLICT_OFFSET = 3

const LOG_BT_ORIGINAL = "Original"
const LOG_BT_BIN_EXP = "Binary Exponential"
const LOG_BT_TERM_BYPASS = "Conflict Term Bypassing"
const LOG_BT_AGGRESSIVE = "Super Aggressive"

// choose one log backtracking mode above
const LOG_BACKTRACKING_MODE = LOG_BT_BIN_EXP

// ENABLE_LOG turn this flag off when testing for performance
const ENABLE_LOG = false

// LOG_TO_FILE log to file or console
const LOG_TO_FILE = true

const ENABLE_TEST_VERBOSE = false

const ENABLE_TRACE_ID = false

const ENABLE_SNAPSHOT_ID = false

const ENABLE_DEBUG_FAST_FAIL = false

const GOROUTINE_NUM_LOG_FILENAME = "goroutine_num_xxafbnlassfb.log"

/////////////////////////// follower parameters ///////////////////////////////

const ELECTION_TIMEOUT_MIN_TIME_MS = 330
const ELECTION_TIMEOUT_MAX_TIME_MS = 650

/////////////////////////// leader parameters /////////////////////////////////

// HEARTBEAT_FREQUENCY "the tester limits you to 10 heartbeats per second"
const HEARTBEAT_FREQUENCY = time.Millisecond * 107

// ENABLE_START_SEND_AE whether the leader will send AE RPC immediately when Start() is called
const ENABLE_START_SEND_AE = true

const AE_CONFLICT_RETRIES = 5

///////////////////////////////////////////////////////////////////////////////

var configLock sync.Mutex
var logInited = false

func configLog() {
	configLock.Lock()
	defer configLock.Unlock()
	if logInited {
		return
	}
	logInited = true
	if !ENABLE_LOG {
		log.SetOutput(ioutil.Discard)
		return
	}
	log.SetFlags(log.Ltime | log.Lmicroseconds | log.Lshortfile)
	if LOG_TO_FILE {
		file, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal("fail to open app.log", err)
		}
		log.SetOutput(file)
	}
}

func validateLogBacktrackingMode() {
	switch LOG_BACKTRACKING_MODE {
	case LOG_BT_ORIGINAL:
	case LOG_BT_BIN_EXP:
	case LOG_BT_TERM_BYPASS:
	case LOG_BT_AGGRESSIVE:
	default:
		log.Fatal("invalid log backtracking mode")
	}

	log.Printf("log backtracking mode: %s", LOG_BACKTRACKING_MODE)

	if LOG_BACKTRACKING_MODE == LOG_BT_TERM_BYPASS {
		log.Fatalf("unfortunately, this mode is not supported after lab2c. You should switch to other mode")
	}
}

var cgntLock sync.Mutex
var cgntInited bool

func startCheckGoroutineNumTicker() {
	cgntLock.Lock()
	defer cgntLock.Unlock()
	if cgntInited {
		return
	}
	cgntInited = true
	log.Printf("start CheckGoroutineNumTicker")

	if err := os.Remove(GOROUTINE_NUM_LOG_FILENAME); err != nil {
		log.Printf("Failed to remove old log file: %v", err)
	}

	file, err := os.Create(GOROUTINE_NUM_LOG_FILENAME)
	if err != nil {
		fmt.Printf("Failed to create log file: %v", err)
		return
	}

	go func() {
		for {
			time.Sleep(time.Second * 1)
			num := runtime.NumGoroutine()
			log.Printf("Current Goroutine Num: %d", num)
			if _, err := file.WriteString(fmt.Sprintf("%d\n", num)); err != nil {
				fmt.Printf("Failed to write to log file: %v", err)
			}
		}
	}()
}
