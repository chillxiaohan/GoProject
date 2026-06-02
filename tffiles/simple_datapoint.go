package main

import (
	"encoding/binary"
	"math"
)

// SimpleDataPoint 简化的灵活数据点（全部float32）
type SimpleDataPoint struct {
	Timestamp int64     // 时间戳（纳秒）
	Values    []float32 // 动态数量的float32值（4字节/个）
}

// EncodeSimplePoint 编码（float32版本）
// 格式: [时间戳:8字节] [字段数量:2字节] [值1:4字节] [值2:4字节] ...
// 总大小 = 8 + 2 + N*4 字节
func EncodeSimplePoint(point SimpleDataPoint) []byte {
	// 8 + 2 + N*4 字节
	buf := make([]byte, 10+len(point.Values)*4)

	// 写入时间戳（8字节）
	binary.LittleEndian.PutUint64(buf[0:8], uint64(point.Timestamp))

	// 写入字段数量（2字节）
	binary.LittleEndian.PutUint16(buf[8:10], uint16(len(point.Values)))

	// 写入所有值（每个4字节）
	for i, val := range point.Values {
		binary.LittleEndian.PutUint32(buf[10+i*4:10+(i+1)*4], math.Float32bits(val))
	}

	return buf
}

// DecodeSimplePoint 解码（float32版本）
func DecodeSimplePoint(data []byte) (*SimpleDataPoint, error) {
	if len(data) < 10 {
		return nil, nil
	}

	// 读取时间戳
	timestamp := int64(binary.LittleEndian.Uint64(data[0:8]))

	// 读取字段数量
	fieldCount := int(binary.LittleEndian.Uint16(data[8:10]))

	// 检查数据长度
	expectedLen := 10 + fieldCount*4
	if len(data) < expectedLen {
		return nil, nil
	}

	// 读取所有值（float32）
	values := make([]float32, fieldCount)
	for i := 0; i < fieldCount; i++ {
		bits := binary.LittleEndian.Uint32(data[10+i*4 : 10+(i+1)*4])
		values[i] = math.Float32frombits(bits)
	}

	return &SimpleDataPoint{
		Timestamp: timestamp,
		Values:    values,
	}, nil
}

// 字段索引常量（6字段设备）
const (
	IdxTravel        = 0
	IdxPitch         = 1
	IdxRotation      = 2
	IdxStockStatus   = 3
	IdxReclaimStatus = 4
	IdxPLCStatus     = 5
)

// CreateDevicePoint 创建设备数据点（6个字段，float32版本）
func CreateDevicePoint(timestamp int64, travel, pitch, rotation float32, stockStatus, reclaimStatus, plcStatus int32) SimpleDataPoint {
	return SimpleDataPoint{
		Timestamp: timestamp,
		Values: []float32{
			travel,
			pitch,
			rotation,
			float32(stockStatus),
			float32(reclaimStatus),
			float32(plcStatus),
		},
	}
}

// ParseDevicePoint 解析设备数据点（float32版本）
func ParseDevicePoint(point *SimpleDataPoint) (travel, pitch, rotation float32, stockStatus, reclaimStatus, plcStatus int32) {
	if len(point.Values) >= 6 {
		travel = point.Values[IdxTravel]
		pitch = point.Values[IdxPitch]
		rotation = point.Values[IdxRotation]
		stockStatus = int32(point.Values[IdxStockStatus])
		reclaimStatus = int32(point.Values[IdxReclaimStatus])
		plcStatus = int32(point.Values[IdxPLCStatus])
	}
	return
}

// ToMap 转换为map（便于使用）
func (point *SimpleDataPoint) ToMap(fieldNames []string) map[string]float32 {
	result := make(map[string]float32)
	for i, name := range fieldNames {
		if i < len(point.Values) {
			result[name] = point.Values[i]
		}
	}
	return result
}
