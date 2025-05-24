package mr

import (
	"log"
	"os"
	"sync"
	"time"
)

///////////////////////////// server parameters ///////////////////////////////////////
const SERVER_CHECK_TIMEOUT_ASSIGNED_TASKS_FREQUENCY = time.Second * 2
const ASSIGNED_TASKS_TIMEOUT = time.Second * 10

const FIN_WAIT_DURATION = 10 * time.Second

//////////////////////////// worker parameters ////////////////////////////////////////
const WORKER_PING_FREQUENCY = time.Second

// increase countdown if ping frequency is small
const WORKER_EXIT_COUNTDOWN = 5

//////////////////////////// shared paramaters //////////////////////////////////////////
const TEMP_FOLDER = "tmp"

// turn this on when using test-mr.sh
const LOG_TO_FILE = true

//////////////////////////////////////////////////////////////////////////////////////////

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
