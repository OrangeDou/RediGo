package client

import (
	"bufio"
	"context"
	"io"
	"net"
	"redigo/utils"

	"sync"
	"time"
)

var (
	logger utils.Logger
)

type Client struct {
	Conn    net.Conn
	Waiting utils.Wait
}

type EchoHandler struct {
	// 保存所有工作状态client的集合(把map当set用)
	// 需使用并发安全的容器
	activeConn sync.Map

	// 关闭状态标识位
	closing utils.AtomicBool
}

func MakeEchoHandler() *EchoHandler {
	return &EchoHandler{}
}

func (h *EchoHandler) Handler(ctx context.Context, conn net.Conn) {
	if h.closing.Load() {
		conn.Close()
		return
	}

	client := &Client{
		Conn: conn,
	}
	//记住仍然存活的连接
	h.activeConn.Store(client, struct{}{})
	//新建读缓冲区
	reader := bufio.NewReader(conn)
	for {
		msg, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				logger.Info("connection close")
				h.activeConn.Delete(client)
			} else {
				logger.Error(err.Error())
			}
			return
		}
		//发送数据前置于waiting状态，防止连接被关闭
		client.Waiting.Add(1)

		b := []byte(msg)
		conn.Write(b)
		client.Conn.Close()
	}
}

// close the client connection
func (c *Client) Close() error {
	c.Waiting.WaitWithTimeout(10 * time.Second)
	c.Conn.Close()
	return nil
}

// close the server
func (h *EchoHandler) Close() error {
	logger.Info("handler shutting down...")
	h.closing.Store(true)
	//逐个关闭连接
	h.activeConn.Range(func(key, value any) bool {
		client := key.(*Client)
		client.Close()
		return true
	})
	return nil
}
