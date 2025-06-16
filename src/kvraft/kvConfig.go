package kvraft

import "time"

/////////////////////////// shared parameters /////////////////////////////////

const TICKER_FREQUENCY = 1 * time.Millisecond

// ENABLE_LOG_VALUE value may be too large to log
const ENABLE_LOG_VALUE = false

const ENABLE_TEST_VERBOSE = true

/////////////////////////// client parameters /////////////////////////////////

const ENABLE_QUERY_SERVER_STATUS = true
const QUERY_SERVER_STATUS_FREQUENCY = 300 * time.Millisecond

const REQUEST_TIMEOUT = 1000 * time.Millisecond
