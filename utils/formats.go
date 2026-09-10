package utils

import (
	"fmt"
	"strings"
	"time"
)

// FormatBytes 格式化字节
func FormatBytes(bytes uint64) string {
	const unit = uint64(1024)
	const sizeUnits = "KMGTPE"
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := unit, uint64(0)
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	format := fmt.Sprintf("%.2f %ciB", float64(bytes)/float64(div), sizeUnits[exp])
	return strings.ReplaceAll(format, ".00", "")
}

// durationReplacer 时间单位替换
var durationReplacer = strings.NewReplacer(
	"ns", "纳秒", "µs", "微秒", "ms", "毫秒", "s", "秒", "m", "分", "h", "时", "d", "天", "w", "周", "y", "年",
)

// FormatDuration 格式化时间
func FormatDuration(t time.Duration) string {
	return durationReplacer.Replace(t.String())
}
