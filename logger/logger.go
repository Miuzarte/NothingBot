package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

var (
	lastLogOutMonth int
	lastLogOutDay   int
	lastLogOutMu    sync.Mutex
)

func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	AddSink(ConsoleSink(os.Stderr, true))
}

// fanout 把同一份 JSON 分发给所有 sink, 支持运行期追加输出 (文件日志)
type fanout struct{}

var (
	sinksMu sync.RWMutex
	sinks   []io.Writer
)

func (fanout) Write(p []byte) (n int, err error) {
	sinksMu.RLock()
	defer sinksMu.RUnlock()
	for _, w := range sinks {
		if _, err = w.Write(p); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// AddSink 追加一个输出目标, 目标需自行把 JSON 渲染成文本, 见 [ConsoleSink]
func AddSink(w io.Writer) {
	sinksMu.Lock()
	defer sinksMu.Unlock()
	sinks = append(sinks, w)
}

// AddOutput 追加一个无颜色的文本输出, 用于文件日志
func AddOutput(out io.Writer) {
	AddSink(ConsoleSink(out, false))
}

// ConsoleSink 返回把 zerolog 的 JSON 渲染成文本的 writer
//
// color 为 false 时不带 ANSI 颜色, 等级显示为 [INFO] 而不是默认的 INF
func ConsoleSink(out io.Writer, color bool) io.Writer {
	cw := zerolog.ConsoleWriter{
		Out:     out,
		NoColor: !color,
		// scope 放在等级之后, 等价于原先的 "[main] message" 横幅
		PartsOrder: []string{
			zerolog.TimestampFieldName,
			zerolog.LevelFieldName,
			"scope",
			zerolog.MessageFieldName,
		},
		FieldsExclude:   []string{"scope"},
		FormatTimestamp: formatTimestamp,
		// scope 是 PartsOrder 里的部件而非普通字段, 用 [scope] 横幅形式输出
		FormatPartValueByName: func(i any, name string) string {
			s := fmt.Sprintf("%s", i)
			if name == "scope" {
				s = "[" + s + "]"
			}
			return s
		},
	}
	if color {
		cw.FormatLevel = formatLevel
	} else {
		cw.FormatLevel = formatLevelPlain
	}
	return cw
}

// New 返回带 scope 字段的 logger, 输出走全局 sink, 可运行期追加
func New(scope string) zerolog.Logger {
	return zerolog.New(fanout{}).
		With().
		Timestamp().
		Str("scope", scope).
		Logger()
}

// NewWithOutput 返回直接写入 out 的 logger, 不走全局 sink
func NewWithOutput(scope string, out io.Writer) zerolog.Logger {
	return zerolog.New(ConsoleSink(out, true)).
		With().
		Timestamp().
		Str("scope", scope).
		Logger()
}

// SetLevel 设置全局日志等级
func SetLevel(level zerolog.Level) {
	zerolog.SetGlobalLevel(level)
}

// Level 把配置里的 0..6 换算成 zerolog 等级
//
// 配置沿用 SimpleLog 的编号 (值越大输出越少), 即 0=Trace ... 6=Panic,
// 与 zerolog 的 -1..5 相差 1
func Level(n int) zerolog.Level {
	return zerolog.Level(min(max(n, 0), 6)) - 1
}

// FakePanic 打印消息与当前调用栈, 不 panic
// 替代 SimpleLog.FakePanic
func FakePanic(l *zerolog.Logger, a ...any) {
	l.Error().Msg(fmt.Sprint(a...))
	l.Error().Msg(string(debug.Stack()))
}

func formatTimestamp(i any) (s string) {
	var t time.Time

	switch tt := i.(type) {
	case string:
		ts, err := time.ParseInLocation(time.RFC3339, tt, time.Local)
		if err != nil {
			return tt
		}
		t = ts
	case json.Number:
		timestamp, err := tt.Int64()
		if err != nil {
			return tt.String()
		}
		t = time.Unix(timestamp, 0)
	default:
		return "<nil>"
	}

	month, day := int(t.Month()), t.Day()
	lastLogOutMu.Lock()
	defer lastLogOutMu.Unlock()
	if month != lastLogOutMonth || day != lastLogOutDay {
		s = t.Format("[01/02 15:04:05]")
	} else {
		s = t.Format("[15:04:05]")
	}
	lastLogOutMonth, lastLogOutDay = month, day
	return
}

func formatLevel(i any) string {
	var level string
	if ll, ok := i.(string); ok {
		level = ll
	} else {
		return ""
	}

	switch level {
	case zerolog.LevelTraceValue:
		return "\x1b[94m[TRACE]\x1b[m"
	case zerolog.LevelDebugValue:
		return "\x1b[92m[DEBUG]\x1b[m"
	case zerolog.LevelInfoValue:
		return "\x1b[97m [INFO]\x1b[m"
	case zerolog.LevelWarnValue:
		return "\x1b[93m [WARN]\x1b[m"
	case zerolog.LevelErrorValue:
		return "\x1b[91m[ERROR]\x1b[m"
	case zerolog.LevelFatalValue:
		return "\x1b[91;5m[FATAL]\x1b[m"
	case zerolog.LevelPanicValue:
		return "\x1b[91;5;7m[PANIC]\x1b[m"
	default:
		return "[" + level + "]"
	}
}

func formatLevelPlain(i any) string {
	var level string
	if ll, ok := i.(string); ok {
		level = ll
	} else {
		return ""
	}

	switch level {
	case zerolog.LevelTraceValue:
		return "[TRACE]"
	case zerolog.LevelDebugValue:
		return "[DEBUG]"
	case zerolog.LevelInfoValue:
		return " [INFO]"
	case zerolog.LevelWarnValue:
		return " [WARN]"
	case zerolog.LevelErrorValue:
		return "[ERROR]"
	case zerolog.LevelFatalValue:
		return "[FATAL]"
	case zerolog.LevelPanicValue:
		return "[PANIC]"
	default:
		return "[" + level + "]"
	}
}
