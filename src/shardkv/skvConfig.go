package shardkv

import (
	"6.824/atopraft"
	"time"
)

type SKVConfig struct {
	/////////////////////////// shared parameters /////////////////////////////////

	BC *atopraft.BaseConfig

	/////////////////////////// server parameters /////////////////////////////////

	SrvQueryConfigFrequency time.Duration
	SrvRPCFrequency         time.Duration
	// whether to log RPC args and reply
	EnableRPCLog bool
	// whether the result of append operation is returned
	returnValueForAppend bool

	/////////////////////////// client parameters /////////////////////////////////

	CkQueryConfigFrequency time.Duration
}

func makeSKVConfig() *SKVConfig {
	baseConfig := atopraft.NewBaseConfig()
	baseConfig.LogPrefix = "SKV"
	baseConfig.EnableCheckStatusTicker = false
	baseConfig.QueryServerStatusFrequency = 500 * time.Millisecond
	//baseConfig.RequestTimeout = 1000 * time.Millisecond

	return &SKVConfig{
		BC:                      baseConfig,
		SrvQueryConfigFrequency: time.Millisecond * 100,
		SrvRPCFrequency:         time.Millisecond * 20,
		EnableRPCLog:            false,
		returnValueForAppend:    false,
		CkQueryConfigFrequency:  time.Millisecond * 100,
	}
}
