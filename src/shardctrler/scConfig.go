package shardctrler

import (
	"6.824/atopraft"
	"time"
)

const ENABLE_TEST_VERBOSE = true

func makeSCConfig() *atopraft.BaseConfig {
	config := atopraft.NewBaseConfig()
	config.LogPrefix = "SC"
	config.EnableLog = false
	config.EnableCheckStatusTicker = false
	config.QueryServerStatusFrequency = 500 * time.Millisecond
	//config.EnableLogValue = true
	return config
}
