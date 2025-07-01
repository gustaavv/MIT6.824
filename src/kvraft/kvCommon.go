package kvraft

import (
	"6.824/atopraft"
	"fmt"
)

const (
	OP_GET    = "Get"
	OP_PUT    = "Put"
	OP_APPEND = "Append"
)

type KVPayLoad struct {
	Key   string
	Value string
}

func (pl KVPayLoad) String() string {
	return fmt.Sprintf("KVPayLoad{Key:%q, Value:%q}", pl.Key, pl.Value)
}

func (pl KVPayLoad) Clone() atopraft.ArgsPayLoad {
	return &KVPayLoad{
		Key:   pl.Key,
		Value: pl.Value,
	}
}

type KVReplyValue string

func (rv KVReplyValue) String() string {
	return string(rv)
}

func (rv KVReplyValue) Clone() atopraft.ReplyValue {
	return &rv
}
