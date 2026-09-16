package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gobackendkit/db"
	"gobackendkit/dialect"
	"gobackendkit/schema/excelddl"
)

func main() {
	excelPath := flag.String("excel", "", "表结构 Excel 路径（.xlsx）")
	genSample := flag.String("gen-sample", "", "仅生成示例 Excel 到该路径后退出")
	envFile := flag.String("env", "conf/env", "数据库配置文件（KEY=VALUE）")
	dryRun := flag.Bool("dry-run", false, "只打印 DDL，不执行")
	flag.Parse()

	if *genSample != "" {
		out := filepath.Clean(*genSample)
		dir := filepath.Dir(out)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				log.Fatal(err)
			}
		}
		if err := excelddl.WriteSample(out); err != nil {
			log.Fatal(err)
		}
		abs, _ := filepath.Abs(out)
		fmt.Println("已生成示例 Excel 文件:")
		fmt.Println("  ", abs)
		fmt.Println("说明: demo_items 是该文件里的「工作表名称」（Excel 底部标签），不是单独的文件名。")
		fmt.Println("用 Excel/WPS 打开上述 .xlsx，看左下角工作表即可看到 demo_items。")
		return
	}
	if strings.TrimSpace(*excelPath) == "" {
		flag.Usage()
		log.Fatal("请指定 -excel=表结构.xlsx ，或 -gen-sample=路径.xlsx 生成示例")
	}

	cfg, dbType, err := loadEnvDB(*envFile)
	if err != nil {
		log.Fatal(err)
	}
	db.SetDBType(dbType)
	fmt.Printf("配置文件: %s\n", *envFile)
	fmt.Printf("将连接: type=%s host=%s port=%d user=%s dbname=%s\n",
		dbType, cfg.Host, cfg.Port, cfg.User, cfg.DBName)
	if *dryRun {
		fmt.Println("模式: dry-run（只打印 DDL，不会在数据库里建表）")
	} else {
		fmt.Println("模式: 执行建表（请在客户端刷新同一库查看）")
	}

	tables, err := excelddl.ReadFile(*excelPath)
	if err != nil {
		log.Fatal(err)
	}
	if len(tables) == 0 {
		log.Fatal("Excel 中没有可用工作表/字段")
	}

	var conn db.DB
	if !*dryRun {
		conn, err = db.NewDB(db.DBType(dbType), cfg)
		if err != nil {
			log.Fatal("连接数据库失败: ", err)
		}
		defer conn.Close()
	}

	ctx := context.Background()
	d := dialect.Current()
	for _, t := range tables {
		mssqlDDL, err := excelddl.BuildMSSQLDDL(t)
		if err != nil {
			log.Fatalf("表 %s: %v", t.Name, err)
		}
		ddl := d.ConvertDDL(mssqlDDL)
		fmt.Println("----", t.Name, "----")
		fmt.Println(ddl)

		if *dryRun {
			continue
		}
		exists, err := tableExists(ctx, conn, t.Name)
		if err != nil {
			log.Fatalf("检查表 %s: %v", t.Name, err)
		}
		if exists {
			fmt.Println("已存在，跳过创建:", t.Name)
			continue
		}
		if _, err := conn.Exec(ctx, ddl); err != nil {
			log.Fatalf("创建表 %s 失败: %v", t.Name, err)
		}
		fmt.Println("创建成功:", t.Name)
	}
}

func tableExists(ctx context.Context, conn db.DB, name string) (bool, error) {
	meta := dialect.Current().Meta()
	var n int
	err := conn.QueryRow(ctx, meta.TableExists, name).Scan(&n)
	return n > 0, err
}

func loadEnvDB(path string) (db.DBConfig, string, error) {
	cfg := db.DBConfig{
		Host: "127.0.0.1",
		Port: 5432,
		User: "postgres",
		DBName: "postgres",
	}
	dbType := "postgresql"
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, dbType, fmt.Errorf("读取 %s 失败: %w（可先复制 examples/quickstart/conf/env.example）", path, err)
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
		case "ERP_DB_HOST", "DB_HOST":
			cfg.Host = v
		case "ERP_DB_PORT", "DB_PORT":
			if p, err := strconv.Atoi(v); err == nil {
				cfg.Port = p
			}
		case "ERP_DB_USER", "DB_USER":
			cfg.User = v
		case "ERP_DB_PASSWORD", "DB_PASSWORD":
			cfg.Password = v
		case "ERP_DB_NAME", "DB_NAME":
			cfg.DBName = v
		case "ERP_DB_TYPE", "DB_TYPE":
			dbType = strings.ToLower(v)
			if dbType == "postgres" {
				dbType = "postgresql"
			}
		}
	}
	return cfg, dbType, nil
}
