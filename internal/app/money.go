package app

import "github.com/shopspring/decimal"

// 金额在应用内部统一用 decimal 计算，但对外 JSON 仍保持数字类型：
// 默认的 shopspring/decimal 会把金额编码成带引号的字符串，破坏现有 number 契约。
func init() {
	decimal.MarshalJSONWithoutQuotes = true
}
