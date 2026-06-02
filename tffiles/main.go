// main.go
package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 配置存储
	config := DefaultConfig("./data")
	config.HotDays = 3
	config.RetentionDays = 30
	config.CompressLevel = 6
	config.IndexInterval = 64 * 1024
	config.MaxMemoryMB = 512

	// 创建存储
	storage, err := NewStorage(config)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close()

	// 启动TCP服务器（高性能）
	tcpServer := NewTCPServer(storage)
	if err := tcpServer.Start(8081); err != nil {
		log.Fatal(err)
	}
	defer tcpServer.Stop()

	// 可选：同时启动HTTP服务器用于管理
	// go StartAPIServer(storage, "8080")

	log.Println("=== 时序数据库已启动 ===")
	log.Println("TCP服务: localhost:8081 (高性能写入/查询)")
	log.Println("协议: 自定义二进制协议")
	log.Println("特性: 压缩、索引、分层存储")

	// 打印统计
	go func() {
		for range time.Tick(10 * time.Second) {
			stats := tcpServer.GetStats()
			log.Printf("统计: 连接=%d, 写入=%d, 查询=%d, 流量=%.2fMB/%.2fMB",
				stats["active_connections"],
				stats["total_writes"],
				stats["total_queries"],
				stats["bytes_received_mb"],
				stats["bytes_sent_mb"])
		}
	}()

	// 等待退出
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("正在关闭服务...")
}
