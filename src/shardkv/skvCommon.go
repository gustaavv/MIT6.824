package shardkv

import (
	"6.824/atopraft"
	"6.824/shardctrler"
	"fmt"
)

//
// Sharded key/value server.
// Lots of replica groups, each running Raft.
// Shardctrler decides which group serves each shard.
// Shardctrler may change shard assignment from time to time.
//
// You will have to modify these definitions.
//

const (
	OP_GET    = "Get"
	OP_PUT    = "Put"
	OP_APPEND = "Append"
)

const (
	MSG_CONFIG_NUM_MISMATCH = "CONFIG_NUM_MISMATCH"
	MSG_NOT_MY_SHARD        = "NOT_MY_SHARD"
)

type SKVPayLoad struct {
	Key   string
	Value string
}

func (pl SKVPayLoad) String() string {
	return fmt.Sprintf("SKVPayLoad{Key:%q, Value:%q}", pl.Key, pl.Value)
}

func (pl SKVPayLoad) Clone() atopraft.ArgsPayLoad {
	return &SKVPayLoad{
		Key:   pl.Key,
		Value: pl.Value,
	}
}

type SKVReplyValue string

func (rv SKVReplyValue) String() string {
	return string(rv)
}

func (rv SKVReplyValue) Clone() atopraft.ReplyValue {
	return &rv
}

//
// which shard is a key in?
// please use this function,
// and please do not change it.
//
func key2shard(key string) int {
	shard := 0
	if len(key) > 0 {
		shard = int(key[0])
	}
	shard %= shardctrler.NShards
	return shard
}
