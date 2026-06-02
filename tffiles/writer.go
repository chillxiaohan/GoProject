package main

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
)

// IndexEntry 索引条目（36字节）
type IndexEntry struct {
	StartTime int64  // 起始时间
	EndTime   int64  // 结束时间
	Offset    int64  // 文件偏移
	Length    int64  // 压缩块长度（含4字节CRC32）
	CRC32     uint32 // 数据块CRC32校验码
}

// DayWriter 按天写入器
type DayWriter struct {
	file     *os.File
	idxFile  *os.File
	path     string
	config   *StorageConfig
	deviceID string
	date     time.Time

	fileMu   sync.RWMutex // 保护文件读写的并发安全
	buffer   []SimpleDataPoint
	bufferMu sync.Mutex

	blockBuffer *bytes.Buffer
	compressed  *bytes.Buffer

	indexes       []IndexEntry
	currentOffset int64

	flushChan chan struct{}
	closeChan chan struct{}
	wg        sync.WaitGroup

	encoder *zstd.Encoder
	decoder *zstd.Decoder

	batchSize int
	interval  time.Duration
}

// NewDayWriter 创建写入器
func NewDayWriter(filePath string, config *StorageConfig, deviceID string, date time.Time) (*DayWriter, error) {
	// 打开数据文件
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	// 打开索引文件
	idxPath := filePath + ".idx"
	idxFile, err := os.OpenFile(idxPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		file.Close()
		return nil, err
	}

	// 获取当前文件偏移
	offset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		file.Close()
		idxFile.Close()
		return nil, err
	}

	// 创建压缩器 - 根据压缩级别选择
	var encoder *zstd.Encoder
	var encoderErr error

	switch config.CompressLevel {
	case 1:
		encoder, encoderErr = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
	case 2, 3:
		encoder, encoderErr = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	case 4, 5, 6:
		encoder, encoderErr = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBetterCompression))
	case 7, 8, 9:
		encoder, encoderErr = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	default:
		encoder, encoderErr = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	}

	if encoderErr != nil {
		file.Close()
		idxFile.Close()
		return nil, encoderErr
	}

	// 创建解压器
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		encoder.Close()
		file.Close()
		idxFile.Close()
		return nil, err
	}

	w := &DayWriter{
		file:          file,
		idxFile:       idxFile,
		path:          filePath,
		config:        config,
		deviceID:      deviceID,
		date:          date,
		buffer:        make([]SimpleDataPoint, 0, 1000),
		blockBuffer:   &bytes.Buffer{},
		compressed:    &bytes.Buffer{},
		currentOffset: offset,
		flushChan:     make(chan struct{}, 1),
		closeChan:     make(chan struct{}),
		batchSize:     1000,
		interval:      time.Second,
		encoder:       encoder,
		decoder:       decoder,
	}

	w.wg.Add(1)
	go w.flushLoop()

	return w, nil
}

// WriteBatch 批量写入
func (w *DayWriter) WriteBatch(points []SimpleDataPoint) error {
	if len(points) == 0 {
		return nil
	}

	w.bufferMu.Lock()
	defer w.bufferMu.Unlock()

	w.buffer = append(w.buffer, points...)

	if len(w.buffer) >= w.batchSize {
		select {
		case w.flushChan <- struct{}{}:
		default:
		}
	}

	return nil
}

// flushLoop 定时刷盘
func (w *DayWriter) flushLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-w.flushChan:
			w.flush()
		case <-ticker.C:
			w.flush()
		case <-w.closeChan:
			w.flush()
			return
		}
	}
}

// flush 刷盘（压缩+写入+更新索引）
func (w *DayWriter) flush() {
	w.bufferMu.Lock()
	if len(w.buffer) == 0 {
		w.bufferMu.Unlock()
		return
	}

	points := make([]SimpleDataPoint, len(w.buffer))
	copy(points, w.buffer)
	w.buffer = w.buffer[:0]
	w.bufferMu.Unlock()

	// 编码所有数据点
	w.blockBuffer.Reset()
	for _, point := range points {
		encoded := EncodeSimplePoint(point)
		w.blockBuffer.Write(encoded)
	}

	// 计算CRC32校验码
	rawData := w.blockBuffer.Bytes()
	crc := crc32.ChecksumIEEE(rawData)

	// 追加4字节CRC到数据块末尾
	dataWithCRC := make([]byte, len(rawData)+4)
	copy(dataWithCRC, rawData)
	binary.LittleEndian.PutUint32(dataWithCRC[len(rawData):], crc)

	// 检查是否需要压缩
	var dataToWrite []byte
	if w.blockBuffer.Len() >= w.config.IndexInterval {
		w.compressed.Reset()
		compressedData := w.encoder.EncodeAll(dataWithCRC, w.compressed.Bytes())
		dataToWrite = compressedData
	} else {
		dataToWrite = dataWithCRC
	}

	w.fileMu.Lock()
	defer w.fileMu.Unlock()

	// 写入数据文件
	offset := w.currentOffset
	n, err := w.file.Write(dataToWrite)
	if err != nil {
		log.Printf("写入数据文件失败 (%s): %v", w.path, err)
		return
	}

	// 立即sync，确保数据持久化
	if syncErr := w.file.Sync(); syncErr != nil {
		log.Printf("sync数据文件失败 (%s): %v", w.path, syncErr)
	}

	// 更新索引
	if len(points) > 0 {
		idx := IndexEntry{
			StartTime: points[0].Timestamp,
			EndTime:   points[len(points)-1].Timestamp,
			Offset:    offset,
			Length:    int64(n),
			CRC32:     crc,
		}
		w.indexes = append(w.indexes, idx)

		// 写入索引文件（追加）
		idxData := make([]byte, 36)
		binary.LittleEndian.PutUint64(idxData[0:8], uint64(idx.StartTime))
		binary.LittleEndian.PutUint64(idxData[8:16], uint64(idx.EndTime))
		binary.LittleEndian.PutUint64(idxData[16:24], uint64(idx.Offset))
		binary.LittleEndian.PutUint64(idxData[24:32], uint64(idx.Length))
		binary.LittleEndian.PutUint32(idxData[32:36], idx.CRC32)
		w.idxFile.Write(idxData)

		if syncErr := w.idxFile.Sync(); syncErr != nil {
			log.Printf("sync索引文件失败 (%s): %v", w.path, syncErr)
		}
	}

	w.currentOffset += int64(n)
}

// Close 关闭写入器
func (w *DayWriter) Close() error {
	close(w.closeChan)
	w.wg.Wait()

	if w.encoder != nil {
		w.encoder.Close()
	}
	if w.decoder != nil {
		w.decoder.Close()
	}

	var lastErr error
	if w.file != nil {
		if err := w.file.Sync(); err != nil {
			lastErr = err
		}
		if err := w.file.Close(); err != nil {
			lastErr = err
		}
	}

	if w.idxFile != nil {
		if err := w.idxFile.Sync(); err != nil {
			lastErr = err
		}
		if err := w.idxFile.Close(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// GetBufferSize 获取当前缓冲区大小
func (w *DayWriter) GetBufferSize() int {
	w.bufferMu.Lock()
	defer w.bufferMu.Unlock()
	return len(w.buffer)
}

// GetIndexCount 获取索引数量
func (w *DayWriter) GetIndexCount() int {
	return len(w.indexes)
}

// Sync 强制同步文件
func (w *DayWriter) Sync() error {
	w.fileMu.RLock()
	defer w.fileMu.RUnlock()

	if err := w.file.Sync(); err != nil {
		return err
	}
	if err := w.idxFile.Sync(); err != nil {
		return err
	}
	return nil
}

// GetFileSize 获取当前文件大小
func (w *DayWriter) GetFileSize() (int64, error) {
	w.fileMu.RLock()
	defer w.fileMu.RUnlock()

	stat, err := w.file.Stat()
	if err != nil {
		return 0, err
	}
	return stat.Size(), nil
}

// GetStats 获取写入器统计
func (w *DayWriter) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"device_id":      w.deviceID,
		"date":           w.date.Format("2006-01-02"),
		"buffer_size":    w.GetBufferSize(),
		"index_count":    w.GetIndexCount(),
		"current_offset": w.currentOffset,
	}
}
