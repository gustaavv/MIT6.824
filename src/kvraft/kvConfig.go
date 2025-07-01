package kvraft

import "6.824/atopraft"

const ENABLE_TEST_VERBOSE = false

func makeKVConfig() *atopraft.BaseConfig {
	config := atopraft.NewBaseConfig()
	config.LogPrefix = "KV"
	return config
}
