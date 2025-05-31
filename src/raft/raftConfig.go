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

const LOG_TO_FILE = false

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
