package aof

import (
	"os"
	"redigo/interface/redis"
	"redigo/redis/protocol"
	"sync"
)

type DB struct {
	// 主线程使用此channel将要持久化的命令发送到异步协程
	aofChan chan *protocol.MultiBulkReply
	// append file 文件描述符
	aofFile *os.File
	// append file 路径
	aofFilename string

	// aof 重写需要的缓冲区，将在AOF重写一节详细介绍
	aofRewriteChan chan *protocol.MultiBulkReply
	// 在必要的时候使用此字段暂停持久化操作
	pausingAof sync.RWMutex
}

type extra struct {
	// 表示该命令是否需要持久化
	toPersist bool
	// 如上文所述 expire 之类的命令不能直接持久化
	// 若 specialAof == nil 则将命令原样持久化，否则持久化 specialAof 中的指令
	specialAof []*protocol.MultiBulkReply
}

type CmdFunc func(db *DB, args [][]byte) (redis.Reply, *extra)
