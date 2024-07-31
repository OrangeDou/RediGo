package dict

import "sync"

//数据库字典
type ConcurrentDict struct {
	table []*Shard
	count int32
}

//数据库分片
type Shard struct {
	m     map[string]interface{}
	mutex sync.RWMutex
}
