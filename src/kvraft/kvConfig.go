package kvraft

import "time"

/////////////////////////// shared parameters /////////////////////////////////

const TICKER_FREQUENCY = 1 * time.Millisecond
const TICKER_FREQUENCY_LARGE = 500 * time.Millisecond

// ENABLE_LOG_VALUE value may be too large to log
const ENABLE_LOG_VALUE = false

const ENABLE_MD5 = false

const ENABLE_TEST_VERBOSE = false

/////////////////////////// server parameters /////////////////////////////////

const UNAVAILABLE_INDEX_DIFF = 200

/////////////////////////// client parameters /////////////////////////////////

const ENABLE_QUERY_SERVER_STATUS = true

// QUERY_SERVER_STATUS_FREQUENCY this value can be set to 200ms (or even lower)
// on local machine to boost performance.
const QUERY_SERVER_STATUS_FREQUENCY = 300 * time.Millisecond

// REQUEST_TIMEOUT this value can be set to 300ms (or even lower) on local
// machine to boost performance. But when running on GitHub Action, this error
// sometimes happen:
// race: limit on 8128 simultaneously alive goroutines is exceeded, dying
// So, I set it large enough to safely run GitHub Action.
const REQUEST_TIMEOUT = 500 * time.Millisecond

// CACHE_SIZE number of last responses cached. set to -1 to disable this feature
const CACHE_SIZE = 10
const CACHE_TRIM_FREQUENCY = 3000 * time.Millisecond

// CACHE_TRIM_MAX_DURATION each trim cache task can not take too long,
// which will block doing requests
const CACHE_TRIM_MAX_DURATION = 10 * time.Millisecond
