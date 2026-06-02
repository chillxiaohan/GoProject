package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StorageConfig 存储配置
type StorageConfig struct {
	DataDir       string // 数据目录
	HotDays       int    // 热数据保留天数
	RetentionDays int    // 总保留天数
	CompressLevel int    // 压缩级别 1-9
	IndexInterval int    // 索引间隔（字节）
	MaxMemoryMB   int    // 最大内存使用(MB)
}

// DefaultConfig 默认配置
func DefaultConfig(dataDir string) *StorageConfig {
	return &StorageConfig{
		DataDir:       dataDir,
		HotDays:       3,
		RetentionDays: 30,
		CompressLevel: 6,
		IndexInterval: 64 * 1024, // 64KB一个索引点
		MaxMemoryMB:   512,
	}
}

// Storage 存储引擎
type Storage struct {
	config        *StorageConfig
	mu            sync.RWMutex
	writers       map[string]*DayWriter // key: deviceID_date
	bufferPool    sync.Pool
	deviceSchemas map[string][]string // 设备ID -> 字段名称列表
	schemaMu      sync.RWMutex
}

// NewStorage 创建存储实例
func NewStorage(config *StorageConfig) (*Storage, error) {
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建数据目录失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(config.DataDir, "hot"), 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(config.DataDir, "cold"), 0755); err != nil {
		return nil, err
	}

	s := &Storage{
		config:        config,
		writers:       make(map[string]*DayWriter),
		deviceSchemas: make(map[string][]string),
		bufferPool: sync.Pool{
			New: func() interface{} {
				return make([]byte, 0, 4096)
			},
		},
	}

	// 启动清理协程
	go s.cleanupLoop()
	// 启动分层协程
	go s.tieringLoop()

	return s, nil
}

// registerSchema 注册设备schema（首次写入时自动注册）
func (s *Storage) registerSchema(deviceID string, fieldCount int) {
	s.schemaMu.Lock()
	defer s.schemaMu.Unlock()

	if _, exists := s.deviceSchemas[deviceID]; exists {
		return
	}

	// 动态生成字段名（或从配置读取）
	fieldNames := make([]string, fieldCount)
	for i := 0; i < fieldCount; i++ {
		fieldNames[i] = fmt.Sprintf("field_%d", i)
	}
	s.deviceSchemas[deviceID] = fieldNames
}

// getOrCreateWriter 获取或创建写入器
func (s *Storage) getOrCreateWriter(deviceID string, t time.Time) (*DayWriter, error) {
	key := fmt.Sprintf("%s_%s", deviceID, t.Format("2006-01-02"))

	s.mu.RLock()
	writer, exists := s.writers[key]
	s.mu.RUnlock()

	if exists {
		return writer, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if writer, exists = s.writers[key]; exists {
		return writer, nil
	}

	// 确定存储层级
	tier := "hot"
	if time.Since(t).Hours() > float64(s.config.HotDays*24) {
		tier = "cold"
	}

	dir := filepath.Join(s.config.DataDir, tier, deviceID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	filePath := filepath.Join(dir, t.Format("2006-01-02")+".tsdb")
	writer, err := NewDayWriter(filePath, s.config, deviceID, t)
	if err != nil {
		return nil, err
	}

	s.writers[key] = writer
	return writer, nil
}

// WriteBatch 批量写入数据（SimpleDataPoint版本）
func (s *Storage) WriteBatch(deviceID string, points []SimpleDataPoint) error {
	if len(points) == 0 {
		return nil
	}

	// 自动注册schema
	if len(points) > 0 {
		s.registerSchema(deviceID, len(points[0].Values))
	}

	// 按天分组
	groups := make(map[string][]SimpleDataPoint)
	for _, point := range points {
		t := time.Unix(0, point.Timestamp)
		dateKey := t.Format("2006-01-02")
		groups[dateKey] = append(groups[dateKey], point)
	}

	// 写入各组
	for dateKey, groupPoints := range groups {
		t, _ := time.Parse("2006-01-02", dateKey)
		writer, err := s.getOrCreateWriter(deviceID, t)
		if err != nil {
			return err
		}

		if err := writer.WriteBatch(groupPoints); err != nil {
			return err
		}
	}

	return nil
}

// Query 查询数据（返回SimpleDataPoint）
func (s *Storage) Query(deviceID string, startTime, endTime int64) ([]SimpleDataPoint, error) {
	startDate := time.Unix(0, startTime)
	endDate := time.Unix(0, endTime)

	var allPoints []SimpleDataPoint
	var mu sync.Mutex
	var wg sync.WaitGroup

	// 并发查询多天数据
	for d := startDate; d.Before(endDate.Add(24 * time.Hour)); d = d.Add(24 * time.Hour) {
		wg.Add(1)
		go func(date time.Time) {
			defer wg.Done()

			// 先查热数据
			hotPath := filepath.Join(s.config.DataDir, "hot", deviceID, date.Format("2006-01-02")+".tsdb")
			points, _ := s.queryFile(hotPath, startTime, endTime)

			// 再查冷数据
			coldPath := filepath.Join(s.config.DataDir, "cold", deviceID, date.Format("2006-01-02")+".tsdb")
			coldPoints, _ := s.queryFile(coldPath, startTime, endTime)

			points = append(points, coldPoints...)

			mu.Lock()
			allPoints = append(allPoints, points...)
			mu.Unlock()
		}(d)
	}

	wg.Wait()
	return allPoints, nil
}

// queryFile 查询单个文件
func (s *Storage) queryFile(filePath string, startTime, endTime int64) ([]SimpleDataPoint, error) {
	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, nil
	}

	// 先读索引
	idxPath := filePath + ".idx"
	indexes, err := s.readIndex(idxPath)
	if err != nil {
		// 索引不存在或损坏，全量扫描
		return s.fullScan(filePath, startTime, endTime)
	}

	// 使用索引定位
	reader, err := NewDayReader(filePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return reader.QueryWithIndex(startTime, endTime, indexes)
}

// fullScan 全量扫描（降级方案）
func (s *Storage) fullScan(filePath string, startTime, endTime int64) ([]SimpleDataPoint, error) {
	reader, err := NewDayReader(filePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return reader.Query(startTime, endTime)
}

// readIndex 读取索引文件（兼容32字节旧格式和36字节新格式）
func (s *Storage) readIndex(idxPath string) ([]IndexEntry, error) {
	data, err := os.ReadFile(idxPath)
	if err != nil {
		return nil, err
	}

	entrySize := 36
	if len(data)%32 == 0 && len(data)%36 != 0 {
		entrySize = 32
	}

	var indexes []IndexEntry
	for i := 0; i+entrySize <= len(data); i += entrySize {
		var idx IndexEntry
		idx.StartTime = int64(binary.LittleEndian.Uint64(data[i:]))
		idx.EndTime = int64(binary.LittleEndian.Uint64(data[i+8:]))
		idx.Offset = int64(binary.LittleEndian.Uint64(data[i+16:]))
		idx.Length = int64(binary.LittleEndian.Uint64(data[i+24:]))
		if entrySize == 36 {
			idx.CRC32 = binary.LittleEndian.Uint32(data[i+32:])
		}
		indexes = append(indexes, idx)
	}

	return indexes, nil
}

// cleanupLoop 清理过期数据
func (s *Storage) cleanupLoop() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.cleanupExpiredData()
	}
}

// cleanupExpiredData 清理过期数据
func (s *Storage) cleanupExpiredData() {
	cutoff := time.Now().AddDate(0, 0, -s.config.RetentionDays)

	for _, tier := range []string{"hot", "cold"} {
		devices, _ := os.ReadDir(filepath.Join(s.config.DataDir, tier))
		for _, device := range devices {
			if !device.IsDir() {
				continue
			}
			files, _ := os.ReadDir(filepath.Join(s.config.DataDir, tier, device.Name()))
			for _, file := range files {
				if file.IsDir() {
					continue
				}
				// 解析文件名日期
				dateStr := file.Name()[:10] // "2006-01-02.tsdb"
				fileDate, err := time.Parse("2006-01-02", dateStr)
				if err != nil {
					continue
				}
				if fileDate.Before(cutoff) {
					os.RemoveAll(filepath.Join(s.config.DataDir, tier, device.Name(), file.Name()))
					os.RemoveAll(filepath.Join(s.config.DataDir, tier, device.Name(), file.Name()+".idx"))
				}
			}
		}
	}
}

// tieringLoop 数据分层
func (s *Storage) tieringLoop() {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.moveColdData()
	}
}

// moveColdData 移动冷数据（原子操作：先写临时文件，再重命名）
func (s *Storage) moveColdData() {
	hotDir := filepath.Join(s.config.DataDir, "hot")
	coldDir := filepath.Join(s.config.DataDir, "cold")
	cutoff := time.Now().AddDate(0, 0, -s.config.HotDays)

	devices, _ := os.ReadDir(hotDir)
	for _, device := range devices {
		if !device.IsDir() {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(hotDir, device.Name()))
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			dateStr := file.Name()[:10]
			fileDate, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				continue
			}
			if fileDate.Before(cutoff) {
				srcPath := filepath.Join(hotDir, device.Name(), file.Name())
				srcIdxPath := srcPath + ".idx"

				dstDir := filepath.Join(coldDir, device.Name())
				if err := os.MkdirAll(dstDir, 0755); err != nil {
					log.Printf("创建冷数据目录失败: %v", err)
					continue
				}

				// 写入临时文件
				tmpPath := filepath.Join(dstDir, file.Name()+".tmp")
				if err := s.recompressCold(srcPath, tmpPath); err != nil {
					log.Printf("冷压缩失败 (%s): %v", srcPath, err)
					os.Remove(tmpPath)
					continue
				}

				// 原子重命名临时文件为最终文件
				dstPath := filepath.Join(dstDir, file.Name())
				if err := os.Rename(tmpPath, dstPath); err != nil {
					log.Printf("原子重命名失败 (%s -> %s): %v", tmpPath, dstPath, err)
					os.Remove(tmpPath)
					continue
				}

				// 复制索引文件
				srcIdxData, err := os.ReadFile(srcIdxPath)
				if err == nil {
					dstIdxPath := dstPath + ".idx"
					if err := os.WriteFile(dstIdxPath, srcIdxData, 0644); err != nil {
						log.Printf("复制索引文件失败: %v", err)
					}
				}

				// 删除原热数据文件（数据和索引）
				if err := os.Remove(srcPath); err != nil {
					log.Printf("删除热数据文件失败: %v", err)
				}
				if err := os.Remove(srcIdxPath); err != nil {
					log.Printf("删除热数据索引文件失败: %v", err)
				}
			}
		}
	}
}

// recompressCold 重新压缩冷数据（更高级别压缩）
func (s *Storage) recompressCold(srcPath, dstPath string) error {
	// 读取原数据
	reader, err := NewDayReader(srcPath)
	if err != nil {
		return err
	}
	defer reader.Close()

	points, err := reader.Query(0, 1<<62)
	if err != nil {
		return err
	}

	// 使用更高级别压缩写入
	coldConfig := *s.config
	coldConfig.CompressLevel = 9

	writer, err := NewDayWriter(dstPath, &coldConfig, "", time.Time{})
	if err != nil {
		return err
	}
	defer writer.Close()

	return writer.WriteBatch(points)
}

// GetDeviceSchema 获取设备schema
func (s *Storage) GetDeviceSchema(deviceID string) ([]string, bool) {
	s.schemaMu.RLock()
	defer s.schemaMu.RUnlock()
	schema, exists := s.deviceSchemas[deviceID]
	return schema, exists
}

// GetStats 获取存储统计信息
func (s *Storage) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := map[string]interface{}{
		"active_writers": len(s.writers),
		"device_count":   len(s.deviceSchemas),
		"config": map[string]interface{}{
			"data_dir":       s.config.DataDir,
			"hot_days":       s.config.HotDays,
			"retention_days": s.config.RetentionDays,
			"compress_level": s.config.CompressLevel,
			"index_interval": s.config.IndexInterval,
			"max_memory_mb":  s.config.MaxMemoryMB,
		},
	}

	return stats
}

// Close 关闭存储
func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var lastErr error
	for _, writer := range s.writers {
		if err := writer.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}
