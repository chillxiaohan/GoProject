package main

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"log"
	"math"
	"os"

	"github.com/klauspost/compress/zstd"
)

// DayReader 读取器
type DayReader struct {
	file    *os.File
	decoder *zstd.Decoder
}

// NewDayReader 创建读取器
func NewDayReader(filePath string) (*DayReader, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		file.Close()
		return nil, err
	}

	return &DayReader{
		file:    file,
		decoder: decoder,
	}, nil
}

// Query 全量查询（降级方案，不使用索引），包含CRC32校验
func (r *DayReader) Query(startTime, endTime int64) ([]SimpleDataPoint, error) {
	stat, err := r.file.Stat()
	if err != nil {
		return nil, err
	}

	data := make([]byte, stat.Size())
	_, err = r.file.ReadAt(data, 0)
	if err != nil && err != io.EOF {
		return nil, err
	}

	// 尝试解压缩（如果整个文件是单个压缩块）
	decompressed, err := r.decoder.DecodeAll(data, nil)
	if err != nil {
		// 不是压缩数据且不带CRC（兼容旧格式）
		return r.parseLegacyFormat(data, startTime, endTime), nil
	}

	// 新格式：去掉末尾CRC，逐块解析
	if len(decompressed) < 4 {
		return nil, nil
	}
	dataLen := len(decompressed) - 4
	storedCRC := binary.LittleEndian.Uint32(decompressed[dataLen:])
	computedCRC := crc32.ChecksumIEEE(decompressed[:dataLen])

	if storedCRC != computedCRC {
		log.Printf("CRC32校验失败(全量): 期望=%08x, 实际=%08x", storedCRC, computedCRC)
	}

	return r.parsePoints(decompressed[:dataLen], startTime, endTime), nil
}

// QueryWithIndex 使用索引查询（高效查询），包含CRC32校验
func (r *DayReader) QueryWithIndex(startTime, endTime int64, indexes []IndexEntry) ([]SimpleDataPoint, error) {
	var relevantBlocks []IndexEntry
	startIdx := binarySearchIndex(indexes, startTime)

	for i := startIdx; i < len(indexes); i++ {
		idx := indexes[i]
		if idx.StartTime > endTime {
			break
		}
		if idx.StartTime <= endTime && idx.EndTime >= startTime {
			relevantBlocks = append(relevantBlocks, idx)
		}
	}

	if len(relevantBlocks) == 0 {
		return []SimpleDataPoint{}, nil
	}

	var allPoints []SimpleDataPoint

	for _, block := range relevantBlocks {
		raw := make([]byte, block.Length)
		_, err := r.file.ReadAt(raw, block.Offset)
		if err != nil {
			return nil, err
		}

		// 解压缩（如果数据是压缩的）
		decompressed, err := r.decoder.DecodeAll(raw, nil)
		if err != nil {
			// 未压缩块：有可能是旧格式无CRC
			if block.CRC32 == 0 {
				return r.parseLegacyFormat(raw, startTime, endTime), nil
			}
			decompressed = raw
		}

		// 数据块最后4字节是CRC32，前面是数据内容
		if len(decompressed) < 4 {
			log.Printf("数据块太小，无法校验CRC: offset=%d, len=%d", block.Offset, len(decompressed))
			continue
		}
		dataLen := len(decompressed) - 4
		storedCRC := binary.LittleEndian.Uint32(decompressed[dataLen:])
		computedCRC := crc32.ChecksumIEEE(decompressed[:dataLen])

		if storedCRC != computedCRC {
			log.Printf("CRC32校验失败: offset=%d, 期望=%08x, 实际=%08x", block.Offset, storedCRC, computedCRC)
			continue // CRC校验失败跳过该块
		}

		// 去掉末尾4字节CRC再解析数据点
		points := r.parsePoints(decompressed[:dataLen], startTime, endTime)
		allPoints = append(allPoints, points...)
	}

	return allPoints, nil
}

// parseLegacyFormat 解析无CRC的旧格式数据
func (r *DayReader) parseLegacyFormat(data []byte, startTime, endTime int64) []SimpleDataPoint {
	var points []SimpleDataPoint
	offset := 0
	for offset+10 <= len(data) {
		timestamp := int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
		fieldCount := int(binary.LittleEndian.Uint16(data[offset+8 : offset+10]))
		pointLen := 10 + fieldCount*4

		if offset+pointLen > len(data) {
			break
		}

		values := make([]float32, fieldCount)
		for i := 0; i < fieldCount; i++ {
			bits := binary.LittleEndian.Uint32(data[offset+10+i*4 : offset+10+(i+1)*4])
			values[i] = Float32frombits(bits)
		}

		if timestamp >= startTime && timestamp <= endTime {
			points = append(points, SimpleDataPoint{
				Timestamp: timestamp,
				Values:    values,
			})
		}

		offset += pointLen
	}
	return points
}

// parsePoints 解析数据点（不含CRC）
func (r *DayReader) parsePoints(data []byte, startTime, endTime int64) []SimpleDataPoint {
	var points []SimpleDataPoint
	offset := 0
	for offset+10 <= len(data) {
		timestamp := int64(binary.LittleEndian.Uint64(data[offset : offset+8]))
		fieldCount := int(binary.LittleEndian.Uint16(data[offset+8 : offset+10]))
		pointLen := 10 + fieldCount*4

		if offset+pointLen > len(data) {
			break
		}

		values := make([]float32, fieldCount)
		for i := 0; i < fieldCount; i++ {
			bits := binary.LittleEndian.Uint32(data[offset+10+i*4 : offset+10+(i+1)*4])
			values[i] = Float32frombits(bits)
		}

		if timestamp >= startTime && timestamp <= endTime {
			points = append(points, SimpleDataPoint{
				Timestamp: timestamp,
				Values:    values,
			})
		}

		offset += pointLen
	}
	return points
}

// 如果 Float32frombits 未定义，添加：
func Float32frombits(bits uint32) float32 {
	return math.Float32frombits(bits)
}

// binarySearchIndex 二分查找索引
func binarySearchIndex(indexes []IndexEntry, targetTime int64) int {
	left, right := 0, len(indexes)-1

	for left <= right {
		mid := (left + right) / 2
		if indexes[mid].EndTime < targetTime {
			left = mid + 1
		} else if indexes[mid].StartTime > targetTime {
			right = mid - 1
		} else {
			for mid > 0 && indexes[mid-1].EndTime >= targetTime {
				mid--
			}
			return mid
		}
	}

	return left
}

// Close 关闭读取器
func (r *DayReader) Close() error {
	var lastErr error

	if r.decoder != nil {
		r.decoder.Close()
	}

	if r.file != nil {
		if err := r.file.Close(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// GetFileSize 获取文件大小
func (r *DayReader) GetFileSize() (int64, error) {
	stat, err := r.file.Stat()
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}
