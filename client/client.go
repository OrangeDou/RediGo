package client

import (
	"context"
	"net"
	"redigo/interface/redis"
	"redigo/protocol"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"redigo/lib/sync/wait"

	log "redigo/lib/logger"

	"github.com/hdt3213/godis/redis/parser"
)

const (
	chanSize = 256
	maxWait  = 3 * time.Second
)

type Client struct {
	conn        net.Conn      // TCP
	sendingReqs chan *Request // 等待发送的请求
	waitingReqs chan *Request // 等待服务器响应的请求
	ticker      *time.Ticker  //触发心跳包的计时器
	addr        string

	ctx        context.Context
	cancelFunc context.CancelFunc
	working    *sync.WaitGroup // 有请求正在处理不能立即停止，用于实现 graceful shutdown

}

type Request struct {
	id        uint64      // 请求id
	args      [][]byte    // 上行参数
	reply     redis.Reply // 收到的返回值
	heartbeat bool        // 标记是否是心跳请求
	waiting   *wait.Wait  // 调用协程发送请求后通过 waitgroup 等待请求异步处理完成
	err       error
}

// 调用者将请求发送给后台协程，并通过 wait group 等待异步处理完成
func (client *Client) Send(args [][]byte) redis.Reply {
	request := &Request{
		args:      args,
		heartbeat: false,
		waiting:   &wait.Wait{},
	}
	request.waiting.Add(1)
	client.working.Add(1)
	defer client.working.Done()

	client.sendingReqs <- request
	timeout := request.waiting.WaitWithTimeout(maxWait) // 等待响应或者超时
	if timeout {
		return protocol.MakeErrReply("server time out")
	}
	if request.err != nil {
		return protocol.MakeErrReply("request failed")
	}
	return request.reply

}

// 写协程入口
func (client *Client) handlerWrite() {
	for req := range client.sendingReqs {
		client.doRequest(req)
	}
}

// 发送请求
func (client *Client) doRequest(req *Request) {
	if req == nil || len(req.args) == 0 {
		return
	}
	// 序列化请求
	re := protocol.MakeMultiBulkReply(req.args)
	bytes := re.ToBytes()
	_, err := client.conn.Write(bytes)
	i := 0
	// 失败重试
	for err != nil && i < 3 {
		_, err = client.conn.Write(bytes)
		if err == nil ||
			(!strings.Contains(err.Error(), "timeout") && // only retry timeout
				!strings.Contains(err.Error(), "deadline exceeded")) {
			break
			i++
		}
		if err == nil {
			// 发送成功等待服务器响应
			client.waitingReqs <- req
		} else {
			req.err = err
			req.waiting.Done()
		}
	}
}

// 收到服务端的响应
func (client *Client) finishRequest(reply redis.Reply) {
	defer func() {
		if err := recover(); err != nil {
			debug.PrintStack()
			log.Error(err)
		}
	}()
	request := <-client.waitingReqs
	if request == nil {
		return
	}
	request.reply = reply
	if request.waiting != nil {
		request.waiting.Done()
	}
}

// 读协程是个 RESP 协议解析器
func (client *Client) handleRead() error {
	ch := parser.ParseStream(client.conn)
	for payload := range ch {
		if payload.Err != nil {
			client.finishRequest(protocol.MakeErrReply(payload.Err.Error()))
			continue
		}
		client.finishRequest(payload.Data)
	}
	return nil
}

// client构造器
func MakeClient(addr string) (*Client, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		addr:        addr,
		conn:        conn,
		sendingReqs: make(chan *Request, chanSize),
		waitingReqs: make(chan *Request, chanSize),
		ctx:         ctx,
		cancelFunc:  cancel,
		working:     &sync.WaitGroup{},
	}, nil
}

func (client *Client) Start() {
	client.ticker = time.NewTicker(10 * time.Second)
	go client.handleWrite()
	go func() {
		err := client.handleRead()
		log.Warn(err)
	}()
	go client.heartbeat()
}
func (client *Client) Close() {
	// 先阻止新请求进入队列
	close(client.sendingReqs)

	// 等待处理中的请求完成
	client.working.Wait()

	// 释放资源
	_ = client.conn.Close()   // 关闭与服务端的连接，连接关闭后读协程会退出
	client.cancelFunc()       // 使用 context 关闭读协程
	close(client.waitingReqs) // 关闭队列
}

func (client *Client) handleWrite() {
	for req := range client.sendingReqs {
		client.doRequest(req)
	}
}

func (client *Client) heartbeat() {
	for range client.ticker.C {
		client.doHeartbeat()
	}
}
func (client *Client) doHeartbeat() {
	request := &Request{
		args:      [][]byte{[]byte("PING")},
		heartbeat: true,
		waiting:   &wait.Wait{},
	}
	request.waiting.Add(1)
	client.working.Add(1)
	defer client.working.Done()
	client.sendingReqs <- request
	request.waiting.WaitWithTimeout(maxWait)
}
