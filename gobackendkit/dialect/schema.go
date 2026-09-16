package dialect

// Schema 元数据查询（information_schema），按方言返回不同 SQL。
type Schema struct {
	TableExists  string
	ColumnExists string
}

// CurrentMeta 当前方言元数据查询。
func CurrentMeta() Schema {
	return Current().Meta()
}
