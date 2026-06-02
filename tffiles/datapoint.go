package main

import (
	"encoding/binary"
)

// DataPoint 数据点结构
// 一个数据点占用16字节（8字节时间戳 + 8字节浮点值）
type DataPoint struct {
	Time  int64   // 纳秒级时间戳
	Value float64 // 浮点数值
}

// encodePoint 编码数据点为二进制格式（16字节）
func encodePoint(point DataPoint) []byte {
	buf := make([]byte, 16)
	binary.LittleEndian.PutUint64(buf[0:8], uint64(point.Time))
	binary.LittleEndian.PutUint64(buf[8:16], uint64(point.Value))
	return buf
}

// decodePoint 从二进制数据解码数据点
func decodePoint(data []byte) DataPoint {
	if len(data) < 16 {
		return DataPoint{}
	}
	time := int64(binary.LittleEndian.Uint64(data[0:8]))
	value := float64(binary.LittleEndian.Uint64(data[8:16]))
	return DataPoint{Time: time, Value: value}
}

// BatchPoints 批量数据点（用于高效传输）
type BatchPoints struct {
	DeviceID string
	Points   []DataPoint
}
