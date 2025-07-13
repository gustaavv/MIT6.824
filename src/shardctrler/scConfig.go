package shardctrler

import "6.824/atopraft"

const ENABLE_TEST_VERBOSE = true

func makeSCConfig() *atopraft.BaseConfig {
	config := atopraft.NewBaseConfig()
	config.LogPrefix = "SC"
	config.EnableLog = false
	return config
}
