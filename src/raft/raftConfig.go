package raft

import (
	"log"
	"os"
	"sync"
	"time"
)

/////////////////////////// shared parameters /////////////////////////////////

const STATE_FOLLOWER = "follower"
const STATE_LEADER = "leader"
const STATE_CANDIDATE = "candidate"

const TICKER_FREQUENCY = time.Millisecond * 1

const APPLY_LOGENTRY_FREQUENCY = time.Millisecond * 1

const LOG_BT_ORIGINAL = 1
const LOG_BT_BIN_EXP = 2
const LOG_BT_TERM_BYPASS = 3
const LOG_BT_AGGRESSIVE = 4

// choose one log backtracking mode above
const LOG_BACKTRACKING_MODE = LOG_BT_TERM_BYPASS

const LOG_TO_FILE = true

const ENABLE_TEST_VERBOSE = false

/////////////////////////// follower parameters ///////////////////////////////

const ELECTION_TIMEOUT_MIN_TIME_MS = 500
const ELECTION_TIMEOUT_MAX_TIME_MS = 1000

/////////////////////////// leader parameters /////////////////////////////////

// "the tester limits you to 10 heartbeats per second"
const HEARTBEAT_FERQUENCY = time.Millisecond * 107

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
		log.Print("log backtracking mode: original")
	case LOG_BT_BIN_EXP:
		log.Print("log backtracking mode: binary exponential")
	case LOG_BT_TERM_BYPASS:
		log.Print("log backtracking mode: conflict term bypassing")
	case LOG_BT_AGGRESSIVE:
		log.Print("log backtracking mode: super aggressive")
	default:
		log.Fatal("invalid log backtracking mode")
	}
}
