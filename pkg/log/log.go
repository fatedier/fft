package log

import (
	"os"

	"github.com/fatedier/golib/log"
)

var (
	TraceLevel = log.TraceLevel
	DebugLevel = log.DebugLevel
	InfoLevel  = log.InfoLevel
	WarnLevel  = log.WarnLevel
	ErrorLevel = log.ErrorLevel
)

var StdLogger *log.Logger

func init() {
	StdLogger = log.New(
		log.WithCaller(true),
		log.AddCallerSkip(1),
		log.WithLevel(log.InfoLevel),
	)
}

func InitLog(logWay string, logFile string, logLevel string, maxdays int64) {
	SetLogFile(logWay, logFile, maxdays)
	SetLogLevel(logLevel)
}

// logWay: file or console
func SetLogFile(logWay string, logFile string, maxdays int64) {
	options := []log.Option{}
	if logWay == "console" {
		options = append(options,
			log.WithOutput(log.NewConsoleWriter(log.ConsoleConfig{
				Colorful: true,
			}, os.Stdout)),
		)
	} else {
		writer := log.NewRotateFileWriter(log.RotateFileConfig{
			FileName: logFile,
			Mode:     log.RotateFileModeDaily,
			MaxDays:  int(maxdays),
		})
		writer.Init()
		options = append(options, log.WithOutput(writer))
	}
	StdLogger = StdLogger.WithOptions(options...)
}

// value: error, warning, info, debug, trace
func SetLogLevel(logLevel string) {
	level, err := log.ParseLevel(logLevel)
	if err != nil {
		level = log.WarnLevel // default to warning
	}
	StdLogger = StdLogger.WithOptions(log.WithLevel(level))
}

// wrap log

func Error(format string, v ...interface{}) {
	StdLogger.Errorf(format, v...)
}

func Warn(format string, v ...interface{}) {
	StdLogger.Warnf(format, v...)
}

func Info(format string, v ...interface{}) {
	StdLogger.Infof(format, v...)
}

func Debug(format string, v ...interface{}) {
	StdLogger.Debugf(format, v...)
}

func Trace(format string, v ...interface{}) {
	StdLogger.Tracef(format, v...)
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
	StdLogger.Errorf(pl.prefix+format, v...)
}

func (pl *PrefixLogger) Warn(format string, v ...interface{}) {
	StdLogger.Warnf(pl.prefix+format, v...)
}

func (pl *PrefixLogger) Info(format string, v ...interface{}) {
	StdLogger.Infof(pl.prefix+format, v...)
}

func (pl *PrefixLogger) Debug(format string, v ...interface{}) {
	StdLogger.Debugf(pl.prefix+format, v...)
}

func (pl *PrefixLogger) Trace(format string, v ...interface{}) {
	StdLogger.Tracef(pl.prefix+format, v...)
}
