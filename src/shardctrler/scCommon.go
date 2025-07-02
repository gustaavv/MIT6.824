package shardctrler

import (
	"6.824/atopraft"
	"fmt"
	"strings"
)

//
// Shard controler: assigns shards to replication groups.
//
// RPC interface:
// Join(servers) -- add a set of groups (gid -> server-list mapping).
// Leave(gids) -- delete a set of groups.
// Move(shard, gid) -- hand off one shard from current owner to gid.
// Query(num) -> fetch Config # num, or latest config if num==-1.
//
// A Config (configuration) describes a set of replica groups, and the
// replica group responsible for each shard. Configs are numbered. Config
// #0 is the initial configuration, with no groups and all shards
// assigned to group 0 (the invalid group).
//
// You will need to add fields to the RPC argument structs.
//

// NShards The number of shards.
const NShards = 10

// Config A configuration -- an assignment of shards to groups.
// Please don't change this.
type Config struct {
	Num    int              // config number
	Shards [NShards]int     // shard -> gid
	Groups map[int][]string // gid -> servers[]
}

func (c Config) Clone() Config {
	ans := Config{}

	ans.Num = c.Num

	ans.Shards = [NShards]int{}
	for i, v := range c.Shards {
		ans.Shards[i] = v
	}

	ans.Groups = make(map[int][]string)
	for k, v := range c.Groups {
		ans.Groups[k] = make([]string, len(v))
		copy(ans.Groups[k], v)
	}

	return ans
}

func (c Config) String() string {
	assignMap := make(map[int][]int)

	// this map also includes groups with no shard assigned
	for gid := range c.Groups {
		assignMap[gid] = make([]int, 0)
	}
	for shard, gid := range c.Shards {
		assignMap[gid] = append(assignMap[gid], shard)
	}
	var b strings.Builder
	b.WriteString("{")
	for gid, arr := range assignMap {
		b.WriteString(fmt.Sprintf("%d->%v, ", gid, arr))
	}
	b.WriteString("}")

	return fmt.Sprintf("Config{Num:%v, AssignMap:%s, Shards:%v, GroupLen: %d, Groups:%v}",
		c.Num, b.String(), c.Shards, len(c.Groups), c.Groups)
}

const (
	OP_JOIN  = "Join"
	OP_LEAVE = "Leave"
	OP_MOVE  = "Move"
	OP_QUERY = "Query"
)

type SCPayLoad struct {
	// Join
	Servers map[int][]string
	// Leave
	GIDs []int
	// Move
	Shard int
	GID   int
	// Query
	Num int
}

func (pl SCPayLoad) String() string {
	return fmt.Sprintf("SCPlayLoad{Servers:%v, GIDs:%v, Shard:%d, GID:%d, Num:%d}",
		pl.Servers, pl.GIDs, pl.Shard, pl.GID, pl.Num)
}

func (pl SCPayLoad) Clone() atopraft.ArgsPayLoad {
	ans := new(SCPayLoad)

	ans.Servers = make(map[int][]string)
	for k, v := range pl.Servers {
		ans.Servers[k] = make([]string, len(v))
		copy(ans.Servers[k], v)
	}

	ans.GIDs = make([]int, len(pl.GIDs))
	copy(ans.GIDs, pl.GIDs)

	ans.Shard = pl.Shard
	ans.GID = pl.GID
	ans.Num = pl.Num

	return ans
}

type SCReplyValue struct {
	// Query
	Config Config
}

func (rv SCReplyValue) String() string {
	return fmt.Sprintf("SCReplyValue{Config:%s}", rv.Config)
}

func (rv SCReplyValue) Clone() atopraft.ReplyValue {
	ans := new(SCReplyValue)
	ans.Config = rv.Config.Clone()
	return ans
}
