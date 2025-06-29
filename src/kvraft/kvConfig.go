package kvraft

import "time"

/////////////////////////// shared parameters /////////////////////////////////

const TICKER_FREQUENCY = 1 * time.Millisecond
const TICKER_FREQUENCY_LARGE = 500 * time.Millisecond

// ENABLE_LOG_VALUE value may be too large to log
const ENABLE_LOG_VALUE = false

const ENABLE_MD5 = false

const ENABLE_TEST_VERBOSE = false

/////////////////////////// client parameters /////////////////////////////////

const ENABLE_QUERY_SERVER_STATUS = true
const QUERY_SERVER_STATUS_FREQUENCY = 300 * time.Millisecond

const REQUEST_TIMEOUT = 1000 * time.Millisecond

// CACHE_SIZE number of last responses cached. set to -1 to disable this feature
const CACHE_SIZE = 10
const CACHE_TRIM_FREQUENCY = 3000 * time.Millisecond

// CACHE_TRIM_MAX_DURATION each trim cache task can not take too long,
// which will block doing requests
const CACHE_TRIM_MAX_DURATION = 10 * time.Millisecond
