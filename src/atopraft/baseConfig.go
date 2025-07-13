package atopraft

import "time"

type BaseConfig struct {
	/////////////////////////// shared parameters /////////////////////////////////

	TickerFrequency      time.Duration
	TickerFrequencyLarge time.Duration
	EnableLogging        bool
	// value may be too large to log
	EnableLogValue bool
	EnableMD5      bool
	// to distinguish different services
	LogPrefix string
	EnableLog bool

	/////////////////////////// server parameters /////////////////////////////////

	UnavailableIndexDiff    int
	EnableCheckStatusTicker bool

	/////////////////////////// client parameters /////////////////////////////////

	EnableQueryServerStatus bool
	// this value can be set to 200ms (or even lower)
	// on local machine to boost performance.
	QueryServerStatusFrequency time.Duration
	// this value can be set to 300ms (or even lower) on local
	// machine to boost performance. But when running on GitHub Action, this error
	// sometimes happen:
	// race: limit on 8128 simultaneously alive goroutines is exceeded, dying
	// So, I set it large enough to safely run GitHub Action.
	RequestTimeout time.Duration
	// number of last responses cached. set to -1 to disable this feature
	CacheSize          int
	CacheTrimFrequency time.Duration
	// each trim cache task can not take too long,
	// which will block doing requests
	CacheTrimMaxDuration time.Duration
}

func NewBaseConfig() *BaseConfig {
	return &BaseConfig{
		TickerFrequency:            1 * time.Millisecond,
		TickerFrequencyLarge:       500 * time.Millisecond,
		EnableLogging:              true,
		EnableLogValue:             false,
		EnableMD5:                  false,
		LogPrefix:                  "",
		EnableLog:                  true,
		UnavailableIndexDiff:       200,
		EnableCheckStatusTicker:    true,
		EnableQueryServerStatus:    true,
		QueryServerStatusFrequency: 300 * time.Millisecond,
		RequestTimeout:             500 * time.Millisecond,
		CacheSize:                  10,
		CacheTrimFrequency:         3000 * time.Millisecond,
		CacheTrimMaxDuration:       10 * time.Millisecond,
	}
}
