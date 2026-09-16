// Package gobackendkit 是可复用的 Go 后台基础设施（与具体业务解耦）。
//
// 子包：db（多库连接）、dialect（SQL 方言）、redis（会话/缓存）、auth（可注入鉴权）。
// 任意新项目可直接依赖本 module；本仓库 ERP（erpservices）通过 go.mod replace 融合引用。
//
// 中文教程见：使用教程.md
package gobackendkit
