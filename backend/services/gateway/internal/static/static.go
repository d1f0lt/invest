// Package static содержит файлы, которые gateway отдаёт без авторизации
// (иконки брокеров по /static/brokers/<файл>).
package static

import "embed"

// FS — встроенные в бинарник статические файлы.
//
//go:embed brokers
var FS embed.FS
