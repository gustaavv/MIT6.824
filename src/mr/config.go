package mr

import (
	"log"
	"sync"
	"time"
)

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
}

///////////////////////////// server parameters ///////////////////////////////////////
const SERVER_CKECK_TIMEOUT_ASSIGNED_TASKS_FREQUENCY = time.Second * 2
const ASSIGNED_TASKS_TIMEOUT = time.Second * 10

const FIN_WAIT_DURATION = 10 * time.Second

//////////////////////////// worker parameters ////////////////////////////////////////
const WORKER_PING_FREQUENCY = time.Second

// increase countdown if ping frequency is small
const WORKER_EXIT_COUNTDOWN = 5

//////////////////////////// shared paramaters //////////////////////////////////////////
const TEMP_FOLDER = "tmp"
