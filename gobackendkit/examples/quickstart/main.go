package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gobackendkit/db"
	"gobackendkit/redis"
)

func main() {
	envPath := "conf/env"
	if v := os.Getenv("QUICKSTART_ENV"); v != "" {
		envPath = v
	}
	cfg, dbType, redisAddr, redisPass, redisDB, httpAddr, err := loadConfig(envPath)
	if err != nil {
		log.Fatal(err)
	}

	db.SetDBType(dbType)
	conn, err := db.NewDB(db.DBType(dbType), cfg)
	if err != nil {
		log.Fatal("数据库连接失败: ", err)
	}
	defer conn.Close()
	log.Println("数据库连接成功:", dbType, cfg.Host, cfg.DBName)

	if err := redis.InitRedis(redisAddr, redisPass, redisDB); err != nil {
		log.Fatal("Redis 连接失败: ", err)
	}
	log.Println("Redis 连接成功:", redisAddr, "db=", redisDB)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"message": "quickstart 运行中",
			"time":    time.Now().Format("2006-01-02 15:04:05"),
		})
	})
	mux.HandleFunc("/ping/db", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := conn.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "db": "fail", "error": err.Error()})
			return
		}
		var one int
		if err := conn.QueryRow(ctx, "SELECT 1").Scan(&one); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "db": "fail", "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "db": "ok", "select": one})
	})
	mux.HandleFunc("/ping/redis", func(w http.ResponseWriter, r *http.Request) {
		key := "gobackendkit:quickstart:ping"
		if err := redis.StringSet(key, "1", 30*time.Second); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "redis": "fail", "error": err.Error()})
			return
		}
		val, err := redis.StringGet(key)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "redis": "fail", "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "redis": "ok", "value": val})
	})
	mux.HandleFunc("/demo/items", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := conn.Query(ctx, `
			SELECT id, title, qty FROM demo_items WHERE is_delete = 0 ORDER BY id ASC`)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"ok":      false,
				"message": "查询失败（请先用 excelddl 建表 demo_items）",
				"error":   err.Error(),
			})
			return
		}
		defer rows.Close()
		list := make([]map[string]any, 0)
		for rows.Next() {
			var id, qty int
			var title string
			if err := rows.Scan(&id, &title, &qty); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			list = append(list, map[string]any{"id": id, "title": title, "qty": qty})
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": list, "count": len(list)})
	})

	log.Println("启动 HTTP:", httpAddr)
	log.Println("测试接口: GET /health  /ping/db  /ping/redis  /demo/items")
	log.Fatal(http.ListenAndServe(httpAddr, mux))
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func loadConfig(path string) (db.DBConfig, string, string, string, int, string, error) {
	cfg := db.DBConfig{Host: "127.0.0.1", Port: 5432, User: "postgres", DBName: "postgres"}
	dbType := "postgresql"
	redisAddr := "127.0.0.1:6379"
	redisPass := ""
	redisDB := 0
	httpAddr := ":8090"

	abs, _ := filepath.Abs(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, dbType, redisAddr, redisPass, redisDB, httpAddr,
			fmt.Errorf("读取配置失败 %s: %w\n请复制 conf/env.example 为 conf/env 并填写", abs, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		switch k {
		case "DB_HOST", "ERP_DB_HOST":
			cfg.Host = v
		case "DB_PORT", "ERP_DB_PORT":
			if p, e := strconv.Atoi(v); e == nil {
				cfg.Port = p
			}
		case "DB_USER", "ERP_DB_USER":
			cfg.User = v
		case "DB_PASSWORD", "ERP_DB_PASSWORD":
			cfg.Password = v
		case "DB_NAME", "ERP_DB_NAME":
			cfg.DBName = v
		case "DB_TYPE", "ERP_DB_TYPE":
			dbType = strings.ToLower(v)
			if dbType == "postgres" {
				dbType = "postgresql"
			}
		case "REDIS_ADDR", "ERP_REDIS_ADDR":
			redisAddr = v
		case "REDIS_PASSWORD", "ERP_REDIS_PASSWORD":
			redisPass = v
		case "REDIS_DB", "ERP_REDIS_DB":
			if n, e := strconv.Atoi(v); e == nil {
				redisDB = n
			}
		case "HTTP_ADDR":
			httpAddr = v
		}
	}
	return cfg, dbType, redisAddr, redisPass, redisDB, httpAddr, nil
}
