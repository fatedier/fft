package log

import (
	"fmt"

	"github.com/fatedier/beego/logs"
)

var Log *logs.BeeLogger

func init() {
	Log = logs.NewLogger(200)
	Log.EnableFuncCallDepth(true)
	Log.SetLogFuncCallDepth(Log.GetLogFuncCallDepth() + 1)
}

func InitLog(logWay string, logFile string, logLevel string, maxdays int64) {
	SetLogFile(logWay, logFile, maxdays)
	SetLogLevel(logLevel)
}

// logWay: file or console
func SetLogFile(logWay string, logFile string, maxdays int64) {
	if logWay == "console" {
		Log.SetLogger("console", "")
	} else {
		params := fmt.Sprintf(`{"filename": "%s", "maxdays": %d}`, logFile, maxdays)
		Log.SetLogger("file", params)
	}
}

// value: error, warning, info, debug, trace
func SetLogLevel(logLevel string) {
	level := 4 // warning
	switch logLevel {
	case "error":
		level = 3
	case "warn":
		level = 4
	case "info":
		level = 6
	case "debug":
		level = 7
	case "trace":
		level = 8
	default:
		level = 4
	}
	Log.SetLevel(level)
}

// wrap log

func Error(format string, v ...interface{}) {
	Log.Error(format, v...)
}

func Warn(format string, v ...interface{}) {
	Log.Warn(format, v...)
}

func Info(format string, v ...interface{}) {
	Log.Info(format, v...)
}

func Debug(format string, v ...interface{}) {
	Log.Debug(format, v...)
}

func Trace(format string, v ...interface{}) {
	Log.Trace(format, v...)
}

// Logger
type Logger interface {
	AddLogPrefix(string)
	GetPrefixStr() string
	GetAllPrefix() []string
	ClearLogPrefix()
	Error(string, ...interface{})
	Warn(string, ...interface{})
	Info(string, ...interface{})
	Debug(string, ...interface{})
	Trace(string, ...interface{})
}

type PrefixLogger struct {
	prefix    string
	allPrefix []string
}

func NewPrefixLogger(prefix string) *PrefixLogger {
	logger := &PrefixLogger{
		allPrefix: make([]string, 0),
	}
	logger.AddLogPrefix(prefix)
	return logger
}

func (pl *PrefixLogger) AddLogPrefix(prefix string) {
	if len(prefix) == 0 {
		return
	}

	pl.prefix += "[" + prefix + "] "
	pl.allPrefix = append(pl.allPrefix, prefix)
}

func (pl *PrefixLogger) GetPrefixStr() string {
	return pl.prefix
}

func (pl *PrefixLogger) GetAllPrefix() []string {
	return pl.allPrefix
}

func (pl *PrefixLogger) ClearLogPrefix() {
	pl.prefix = ""
	pl.allPrefix = make([]string, 0)
}

func (pl *PrefixLogger) Error(format string, v ...interface{}) {
	Log.Error(pl.prefix+format, v...)
}

func (pl *PrefixLogger) Warn(format string, v ...interface{}) {
	Log.Warn(pl.prefix+format, v...)
}

func (pl *PrefixLogger) Info(format string, v ...interface{}) {
	Log.Info(pl.prefix+format, v...)
}

func (pl *PrefixLogger) Debug(format string, v ...interface{}) {
	Log.Debug(pl.prefix+format, v...)
}

func (pl *PrefixLogger) Trace(format string, v ...interface{}) {
	Log.Trace(pl.prefix+format, v...)
}
