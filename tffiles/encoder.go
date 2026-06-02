package main

import (
	"encoding/binary"
	"math"
)

// 压缩模式
type CompressionMode int

const (
	CompressNone       CompressionMode = iota
	CompressDelta                      // 增量编码（适合时序数据）
	CompressDeltaDelta                 // 二阶差分（变化缓慢的数据）
)

// 优化编码器
type OptimizedEncoder struct {
	mode CompressionMode
}

func NewOptimizedEncoder(mode CompressionMode) *OptimizedEncoder {
	return &OptimizedEncoder{mode: mode}
}

// 编码批量数据点（压缩版）
func (e *OptimizedEncoder) EncodeBatch(points []DataPoint) []byte {
	if len(points) == 0 {
		return nil
	}

	switch e.mode {
	case CompressDelta:
		return e.encodeDelta(points)
	case CompressDeltaDelta:
		return e.encodeDeltaDelta(points)
	default:
		return e.encodeRaw(points)
	}
}

// 原始编码（16字节/条）
func (e *OptimizedEncoder) encodeRaw(points []DataPoint) []byte {
	buf := make([]byte, len(points)*16)
	for i, p := range points {
		binary.LittleEndian.PutUint64(buf[i*16:], uint64(p.Time))
		binary.LittleEndian.PutUint64(buf[i*16+8:], math.Float64bits(p.Value))
	}
	return buf
}

// 增量编码：存储时间差和值差（约8-12字节/条）
func (e *OptimizedEncoder) encodeDelta(points []DataPoint) []byte {
	if len(points) == 0 {
		return nil
	}

	// 预估大小：基础8字节 + 每条4-8字节
	buf := make([]byte, 0, 8+len(points)*8)

	// 存储第一个点的完整数据
	base := make([]byte, 16)
	binary.LittleEndian.PutUint64(base[0:8], uint64(points[0].Time))
	binary.LittleEndian.PutUint64(base[8:16], math.Float64bits(points[0].Value))
	buf = append(buf, base...)

	// 存储后续点的差值
	var lastTime = points[0].Time
	var lastValue = points[0].Value

	for i := 1; i < len(points); i++ {
		timeDelta := points[i].Time - lastTime
		valueDelta := points[i].Value - lastValue

		// 使用变长编码存储差值
		timeDeltaEncoded := encodeVarint(timeDelta)
		valueDeltaEncoded := encodeVarint(int64(valueDelta * 1000)) // 保留3位小数精度

		// 存储长度标志和差值
		flags := byte(0)
		flags |= byte(len(timeDeltaEncoded)) << 4
		flags |= byte(len(valueDeltaEncoded))
		buf = append(buf, flags)
		buf = append(buf, timeDeltaEncoded...)
		buf = append(buf, valueDeltaEncoded...)

		lastTime = points[i].Time
		lastValue = points[i].Value
	}

	return buf
}

// 二阶差分编码（适合规律变化的数据，约4-8字节/条）
func (e *OptimizedEncoder) encodeDeltaDelta(points []DataPoint) []byte {
	if len(points) == 0 {
		return nil
	}

	buf := make([]byte, 0, 8+len(points)*6)

	// 存储前两个点的完整数据
	for i := 0; i < 2 && i < len(points); i++ {
		base := make([]byte, 16)
		binary.LittleEndian.PutUint64(base[0:8], uint64(points[i].Time))
		binary.LittleEndian.PutUint64(base[8:16], math.Float64bits(points[i].Value))
		buf = append(buf, base...)
	}

	if len(points) <= 2 {
		return buf
	}

	// 计算一阶差分
	firstTimeDelta := points[1].Time - points[0].Time
	firstValueDelta := points[1].Value - points[0].Value

	// 存储一阶差分基准
	buf = append(buf, encodeVarint(firstTimeDelta)...)
	buf = append(buf, encodeVarint(int64(firstValueDelta*1000))...)

	var lastTimeDelta = firstTimeDelta
	var lastValueDelta = firstValueDelta

	// 存储二阶差分
	for i := 2; i < len(points); i++ {
		timeDelta := points[i].Time - points[i-1].Time
		valueDelta := points[i].Value - points[i-1].Value

		secondTimeDelta := timeDelta - lastTimeDelta
		secondValueDelta := valueDelta - lastValueDelta

		// 二阶差分通常很小
		timeEncoded := encodeVarint(secondTimeDelta)
		valueEncoded := encodeVarint(int64(secondValueDelta * 1000))

		flags := byte(0)
		flags |= byte(len(timeEncoded)) << 4
		flags |= byte(len(valueEncoded))
		buf = append(buf, flags)
		buf = append(buf, timeEncoded...)
		buf = append(buf, valueEncoded...)

		lastTimeDelta = timeDelta
		lastValueDelta = valueDelta
	}

	return buf
}

// 解码
func (e *OptimizedEncoder) Decode(data []byte) []DataPoint {
	if len(data) == 0 {
		return nil
	}

	switch e.mode {
	case CompressDelta:
		return e.decodeDelta(data)
	case CompressDeltaDelta:
		return e.decodeDeltaDelta(data)
	default:
		return e.decodeRaw(data)
	}
}

func (e *OptimizedEncoder) decodeRaw(data []byte) []DataPoint {
	count := len(data) / 16
	points := make([]DataPoint, count)
	for i := 0; i < count; i++ {
		points[i].Time = int64(binary.LittleEndian.Uint64(data[i*16:]))
		points[i].Value = math.Float64frombits(binary.LittleEndian.Uint64(data[i*16+8:]))
	}
	return points
}

func (e *OptimizedEncoder) decodeDelta(data []byte) []DataPoint {
	if len(data) < 16 {
		return nil
	}

	points := make([]DataPoint, 0)

	// 解码第一个点
	firstPoint := DataPoint{
		Time:  int64(binary.LittleEndian.Uint64(data[0:8])),
		Value: math.Float64frombits(binary.LittleEndian.Uint64(data[8:16])),
	}
	points = append(points, firstPoint)

	offset := 16
	lastTime := firstPoint.Time
	lastValue := firstPoint.Value

	for offset < len(data) {
		flags := data[offset]
		offset++

		timeLen := int(flags >> 4)
		valueLen := int(flags & 0x0F)

		if offset+timeLen+valueLen > len(data) {
			break
		}

		timeDelta := decodeVarint(data[offset : offset+timeLen])
		valueDelta := float64(decodeVarint(data[offset+timeLen:offset+timeLen+valueLen])) / 1000

		point := DataPoint{
			Time:  lastTime + timeDelta,
			Value: lastValue + valueDelta,
		}
		points = append(points, point)

		lastTime = point.Time
		lastValue = point.Value
		offset += timeLen + valueLen
	}

	return points
}

func (e *OptimizedEncoder) decodeDeltaDelta(data []byte) []DataPoint {
	if len(data) < 32 {
		return nil
	}

	points := make([]DataPoint, 0)

	// 解码前两个点
	point0 := DataPoint{
		Time:  int64(binary.LittleEndian.Uint64(data[0:8])),
		Value: math.Float64frombits(binary.LittleEndian.Uint64(data[8:16])),
	}
	point1 := DataPoint{
		Time:  int64(binary.LittleEndian.Uint64(data[16:24])),
		Value: math.Float64frombits(binary.LittleEndian.Uint64(data[24:32])),
	}
	points = append(points, point0, point1)

	if len(data) <= 32 {
		return points
	}

	offset := 32

	// 读取一阶差分基准
	timeDeltaBase := decodeVarint(data[offset : offset+8])
	offset += 8
	valueDeltaBase := float64(decodeVarint(data[offset:offset+8])) / 1000
	offset += 8

	lastTimeDelta := timeDeltaBase
	lastValueDelta := valueDeltaBase
	lastTime := point1.Time
	lastValue := point1.Value

	for offset < len(data) {
		flags := data[offset]
		offset++

		timeLen := int(flags >> 4)
		valueLen := int(flags & 0x0F)

		if offset+timeLen+valueLen > len(data) {
			break
		}

		secondTimeDelta := decodeVarint(data[offset : offset+timeLen])
		secondValueDelta := float64(decodeVarint(data[offset+timeLen:offset+timeLen+valueLen])) / 1000

		timeDelta := lastTimeDelta + secondTimeDelta
		valueDelta := lastValueDelta + secondValueDelta

		point := DataPoint{
			Time:  lastTime + timeDelta,
			Value: lastValue + valueDelta,
		}
		points = append(points, point)

		lastTimeDelta = timeDelta
		lastValueDelta = valueDelta
		lastTime = point.Time
		lastValue = point.Value
		offset += timeLen + valueLen
	}

	return points
}

// 变长编码（类似protobuf）
func encodeVarint(x int64) []byte {
	ux := uint64(x)
	buf := make([]byte, 0, 10)
	for ux >= 0x80 {
		buf = append(buf, byte(ux)|0x80)
		ux >>= 7
	}
	buf = append(buf, byte(ux))
	return buf
}

func decodeVarint(data []byte) int64 {
	var x uint64
	var s uint
	for i, b := range data {
		if i == 9 {
			x |= uint64(b) << s
			break
		}
		if b < 0x80 {
			x |= uint64(b) << s
			break
		}
		x |= uint64(b&0x7F) << s
		s += 7
	}
	return int64(x)
}
