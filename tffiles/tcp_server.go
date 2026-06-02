package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// 协议常量
const (
	MsgTypeWrite = 0x01
	MsgTypeQuery = 0x02
	MsgTypePong  = 0x03

	RespSuccess = 0x00
	RespError   = 0x01

	ReadBufferSize  = 64 * 1024
	WriteBufferSize = 64 * 1024
	MaxMessageSize  = 10 * 1024 * 1024
)

// TCPServer TCP服务端
type TCPServer struct {
	storage  *Storage
	listener net.Listener
	clients  sync.Map
	stopChan chan struct{}

	activeConns   int64
	totalWrites   int64
	totalQueries  int64
	bytesReceived int64
	bytesSent     int64
}

// TCPClient TCP连接
type TCPClient struct {
	conn     net.Conn
	reader   *bufio.Reader
	writer   *bufio.Writer
	server   *TCPServer
	lastSeen time.Time
}

// NewTCPServer 创建TCP服务器
func NewTCPServer(storage *Storage) *TCPServer {
	return &TCPServer{
		storage:  storage,
		stopChan: make(chan struct{}),
	}
}

// Start 启动TCP服务器
func (s *TCPServer) Start(port int) error {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	s.listener = listener
	log.Printf("TCP服务器启动在 %s", addr)

	go s.acceptLoop()
	return nil
}

// acceptLoop 接受连接循环
func (s *TCPServer) acceptLoop() {
	for {
		select {
		case <-s.stopChan:
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopChan:
				return
			default:
				log.Printf("Accept错误: %v", err)
				continue
			}
		}

		atomic.AddInt64(&s.activeConns, 1)
		go s.handleConnection(conn)
	}
}

// handleConnection 处理连接
func (s *TCPServer) handleConnection(conn net.Conn) {
	defer func() {
		conn.Close()
		atomic.AddInt64(&s.activeConns, -1)
	}()

	// 设置TCP KeepAlive
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(60 * time.Second)
	}

	client := &TCPClient{
		conn:     conn,
		reader:   bufio.NewReaderSize(conn, ReadBufferSize),
		writer:   bufio.NewWriterSize(conn, WriteBufferSize),
		server:   s,
		lastSeen: time.Now(),
	}

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetWriteDeadline(time.Now().Add(60 * time.Second))

	for {
		select {
		case <-s.stopChan:
			return
		default:
		}

		msg, err := client.readMessage()
		if err != nil {
			if err != io.EOF {
				log.Printf("读取消息错误: %v", err)
			}
			return
		}

		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetWriteDeadline(time.Now().Add(60 * time.Second))

		client.handleMessage(msg)
		client.lastSeen = time.Now()
	}
}

// readMessage 读取消息
func (c *TCPClient) readMessage() ([]byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(c.reader, header); err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint32(header[0:4])
	msgType := header[4]

	if length > MaxMessageSize {
		return nil, fmt.Errorf("消息过大: %d", length)
	}

	body := make([]byte, length)
	if _, err := io.ReadFull(c.reader, body); err != nil {
		return nil, err
	}

	atomic.AddInt64(&c.server.bytesReceived, int64(5+length))

	result := make([]byte, 5+len(body))
	copy(result[0:4], header[0:4])
	result[4] = msgType
	copy(result[5:], body)

	return result, nil
}

// handleMessage 处理消息
func (c *TCPClient) handleMessage(msg []byte) {
	if len(msg) < 5 {
		return
	}

	msgType := msg[4]
	body := msg[5:]

	switch msgType {
	case MsgTypeWrite:
		c.handleWrite(body)
	case MsgTypeQuery:
		c.handleQuery(body)
	case MsgTypePong:
		return
	default:
		log.Printf("未知消息类型: %d", msgType)
	}
}

// handleWrite 处理写入请求
func (c *TCPClient) handleWrite(body []byte) {
	startTime := time.Now()

	if len(body) < 6 {
		c.sendErrorWithType(MsgTypeWrite, "消息格式错误")
		return
	}

	deviceIDLen := binary.BigEndian.Uint16(body[0:2])
	if len(body) < int(2+deviceIDLen+4) {
		c.sendErrorWithType(MsgTypeWrite, "消息格式错误")
		return
	}

	deviceID := string(body[2 : 2+deviceIDLen])
	pointCount := binary.BigEndian.Uint32(body[2+deviceIDLen : 6+deviceIDLen])

	points := make([]SimpleDataPoint, 0, pointCount)
	dataOffset := 6 + int(deviceIDLen)

	for i := uint32(0); i < pointCount; i++ {
		if dataOffset+10 > len(body) {
			c.sendErrorWithType(MsgTypeWrite, fmt.Sprintf("数据点%d解析错误: 数据不足", i))
			return
		}

		timestamp := int64(binary.BigEndian.Uint64(body[dataOffset : dataOffset+8]))
		fieldCount := int(binary.BigEndian.Uint16(body[dataOffset+8 : dataOffset+10]))

		pointLen := 10 + fieldCount*4

		if dataOffset+pointLen > len(body) {
			c.sendErrorWithType(MsgTypeWrite, fmt.Sprintf("数据点%d解析错误: 字段数%d超出范围", i, fieldCount))
			return
		}

		values := make([]float32, fieldCount)
		for j := 0; j < fieldCount; j++ {
			bits := binary.BigEndian.Uint32(body[dataOffset+10+j*4 : dataOffset+10+(j+1)*4])
			values[j] = math.Float32frombits(bits)
		}

		points = append(points, SimpleDataPoint{
			Timestamp: timestamp,
			Values:    values,
		})

		dataOffset += pointLen
	}

	if err := c.server.storage.WriteBatch(deviceID, points); err != nil {
		c.sendErrorWithType(MsgTypeWrite, err.Error())
		return
	}

	c.sendResponse(MsgTypeWrite, RespSuccess, nil)
	atomic.AddInt64(&c.server.totalWrites, 1)

	if time.Since(startTime) > 100*time.Millisecond {
		log.Printf("慢写入: %s, %d条, %d字段, 耗时%v",
			deviceID, len(points), len(points[0].Values), time.Since(startTime))
	}
}

// handleQuery 处理查询请求
func (c *TCPClient) handleQuery(body []byte) {
	startTime := time.Now()

	if len(body) < 18 {
		c.sendErrorWithType(MsgTypeQuery, "消息格式错误")
		return
	}

	deviceIDLen := binary.BigEndian.Uint16(body[0:2])
	if len(body) < int(2+deviceIDLen+16) {
		c.sendErrorWithType(MsgTypeQuery, "消息格式错误")
		return
	}

	deviceID := string(body[2 : 2+deviceIDLen])
	startTimeVal := int64(binary.BigEndian.Uint64(body[2+deviceIDLen : 10+deviceIDLen]))
	endTimeVal := int64(binary.BigEndian.Uint64(body[10+deviceIDLen : 18+deviceIDLen]))

	points, err := c.server.storage.Query(deviceID, startTimeVal, endTimeVal)
	if err != nil {
		c.sendErrorWithType(MsgTypeQuery, err.Error())
		return
	}

	totalSize := 4
	for _, point := range points {
		totalSize += 10 + len(point.Values)*4
	}

	responseData := make([]byte, totalSize)
	binary.BigEndian.PutUint32(responseData[0:4], uint32(len(points)))

	respOffset := 4
	for _, point := range points {
		binary.BigEndian.PutUint64(responseData[respOffset:respOffset+8], uint64(point.Timestamp))
		binary.BigEndian.PutUint16(responseData[respOffset+8:respOffset+10], uint16(len(point.Values)))

		for i, val := range point.Values {
			binary.BigEndian.PutUint32(responseData[respOffset+10+i*4:respOffset+10+(i+1)*4], math.Float32bits(val))
		}

		respOffset += 10 + len(point.Values)*4
	}

	c.sendResponse(MsgTypeQuery, RespSuccess, responseData)
	atomic.AddInt64(&c.server.totalQueries, 1)

	elapsed := time.Since(startTime)
	if len(points) > 0 && elapsed > 50*time.Millisecond {
		log.Printf("慢查询: %s, 返回%d条, %d字段, 耗时%v",
			deviceID, len(points), len(points[0].Values), elapsed)
	}
}

// sendResponse 发送响应
func (c *TCPClient) sendResponse(msgType byte, code byte, data []byte) {
	var response []byte

	if len(data) == 0 {
		response = make([]byte, 6)
		binary.BigEndian.PutUint32(response[0:4], 1)
		response[4] = msgType
		response[5] = code
	} else {
		response = make([]byte, 6+len(data))
		binary.BigEndian.PutUint32(response[0:4], uint32(1+len(data)))
		response[4] = msgType
		response[5] = code
		copy(response[6:], data)
	}

	if _, err := c.writer.Write(response); err != nil {
		log.Printf("发送响应失败: %v", err)
		return
	}

	if err := c.writer.Flush(); err != nil {
		log.Printf("刷新缓冲区失败: %v", err)
		return
	}

	atomic.AddInt64(&c.server.bytesSent, int64(len(response)))
}

// sendErrorWithType 发送带正确消息类型的错误响应
func (c *TCPClient) sendErrorWithType(msgType byte, errMsg string) {
	errData := []byte(errMsg)
	response := make([]byte, 6+len(errData))
	binary.BigEndian.PutUint32(response[0:4], uint32(1+len(errData)))
	response[4] = msgType
	response[5] = RespError
	copy(response[6:], errData)

	if _, err := c.writer.Write(response); err != nil {
		log.Printf("发送错误响应失败: %v", err)
		return
	}

	if err := c.writer.Flush(); err != nil {
		log.Printf("刷新缓冲区失败: %v", err)
		return
	}

	atomic.AddInt64(&c.server.bytesSent, int64(len(response)))
}

// Stop 停止服务器
func (s *TCPServer) Stop() {
	close(s.stopChan)
	if s.listener != nil {
		s.listener.Close()
	}
}

// GetStats 获取统计信息
func (s *TCPServer) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"active_connections": atomic.LoadInt64(&s.activeConns),
		"total_writes":       atomic.LoadInt64(&s.totalWrites),
		"total_queries":      atomic.LoadInt64(&s.totalQueries),
		"bytes_received_mb":  float64(atomic.LoadInt64(&s.bytesReceived)) / 1024 / 1024,
		"bytes_sent_mb":      float64(atomic.LoadInt64(&s.bytesSent)) / 1024 / 1024,
	}
}
