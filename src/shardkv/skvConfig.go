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

	/////////////////////////// client parameters /////////////////////////////////

	CkQueryConfigFrequency time.Duration
}

func makeSKVConfig() *SKVConfig {
	baseConfig := atopraft.NewBaseConfig()
	baseConfig.LogPrefix = "SKV"
	baseConfig.EnableCheckStatusTicker = false

	return &SKVConfig{
		BC:                      baseConfig,
		SrvQueryConfigFrequency: time.Millisecond * 25,
		SrvRPCFrequency:         time.Millisecond * 1,
		EnableRPCLog:            false,
		CkQueryConfigFrequency:  time.Millisecond * 10,
	}
}
