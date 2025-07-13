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

	/////////////////////////// client parameters /////////////////////////////////

	CkQueryConfigFrequency time.Duration
}

func makeSKVConfig() *SKVConfig {
	baseConfig := atopraft.NewBaseConfig()
	baseConfig.LogPrefix = "SKV"
	baseConfig.EnableCheckStatusTicker = false

	return &SKVConfig{
		BC:                      baseConfig,
		SrvQueryConfigFrequency: time.Millisecond * 100,
		SrvRPCFrequency:         time.Millisecond * 100,
		CkQueryConfigFrequency:  time.Millisecond * 100,
	}
}
