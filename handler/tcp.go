/*
实现TCP服务的优雅关闭：
由于在生产环境下需要保证TCP服务器关闭前完成必要的清理工作，包括完成正在进行的数据传输，关闭TCP连接，避免资源泄漏以及客户端收到不完整的数据导致故障。
优雅：首先阻止新连接的进入，然后完成正在传输的数据，遍历所有连接挨个关闭。
*/

package handler

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"redigo/utils"
	"sync"
	"syscall"
)

var (
	logger utils.Logger
	viper  viper
)

type Handler interface {
	Handler(ctx context.Context, conn net.Conn)
	Close() error
}
type config struct {
	address string
}

// 功能如其名，监听请求并提供服务，同时收到close信号时关闭
func ListenAndServe(listener net.Listener, handler Handler, closeChan <-chan struct{}) {
	//监听关闭通知
	go func() {
		<-closeChan
		logger.Info("Shart shutting down......")
		listener.Close()
		handler.Close()
	}()
	//异常退出释放资源
	defer func() {
		logger.Info("Something was wrong,shart shutting down......")
		listener.Close()
		handler.Close()
	}()

	context := context.Background()
	var wg *sync.WaitGroup
	for {
		conn, err := listener.Accept()
		if err != nil {
			break
		}
		wg.Add(1)
		go func() {
			handler.Handler(context, conn)
			defer func() {
				wg.Done()
			}()
		}()
	}
	wg.Wait()
}

// 通过监听中断信号，通过closeChan通知服务器关闭
func ListenAndServeWithSignal(cfg *Config, handler Handler) error {
	closeCh := make(chan struct{})
	sigCh := make(chan os.Signal)
	signal.Notify(sigCh, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		sig := <-sigCh
		switch sig {
		case syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			closeCh <- struct{}{}
		}
	}()
	//绑定监听地址
	listener, err := net.Listen("tcp", cfg.address)
	if err != nil {
		log.Fatal(fmt.Sprintf("listen err: %v", err))
	}
	defer listener.Close()
	log.Println(fmt.Sprintf("bind: %s, start listening...", cfg.address))

}
