package mr

import (
	"log"
	"sync"
)

var mu sync.Mutex
var logInited = false

func configLog() {
	mu.Lock()
	defer mu.Unlock()
	if logInited {
		return
	}
	logInited = true
	log.SetFlags(log.Ltime | log.Lmicroseconds | log.Lshortfile)
}
