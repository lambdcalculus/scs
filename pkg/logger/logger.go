package logger

import (
	"fmt"
	"io"
	"os"
	"path"
	"sync"
	"time"
)

type LogLevel int

const (
	LevelTrace LogLevel = iota
	LevelDebug
	LevelInfo
	LevelWarning
	LevelError
	LevelFatal
)

var levelString = map[LogLevel]string{
	LevelTrace:   "  TRACE     ",
	LevelDebug:   "  DEBUG     ",
	LevelInfo:    "  INFO      ",
	LevelWarning: "  WARNING   ",
	LevelError:   "  ERROR  !  ",
	LevelFatal:   "  FATAL !!! ",
}

// A FormatFunc formats meassages into log messages (i.e. by including log levels, timestamps, etc.).
type FormatFunc func(msg string, lvl LogLevel) string

// DefaultFmt formats messages into the form:
// `LEVEL    Mon Jan 2 15:04:05 -0700 2006: message`
// with a new line at the end. It prevents duplication of newlines, if the
// message already has one.
func DefaultFmt(msg string, lvl LogLevel) string {
	// Get time right away.
	logTime := time.Now().Format(time.RubyDate)

	// Don't duplicate newlines.
	if len(msg) > 1 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-2]
	}

	return fmt.Sprintf("%v%v: %v\n", levelString[lvl], logTime, msg)
}

// A Logger logs formatted messages into [io.Writer]s according to their log level.
type Logger struct {
	level   LogLevel
	fmt     FormatFunc
	outputs []io.Writer
	muxs    []sync.Mutex
}

// DefaultLogger logs to stdout and logs at LevelInfo, with a [DefaultFormatter].
var (
	DefaultLogger *Logger = &Logger{
		level:   LevelInfo,
		fmt:     DefaultFmt,
		outputs: []io.Writer{os.Stdout},
		muxs:    make([]sync.Mutex, 1),
	}
	currentLogger *Logger = DefaultLogger
)

// SetLogger sets the logger that will be used on non-method calls.
// Preferably, this is to be set only once, at the top-level.
func SetLogger(logger *Logger) {
	currentLogger = logger
}

// NewLogger creates a logger that logs at the passed level and to
// the passed io.Writer's. It formats messages according to `fmt`.
// If `nil` is passed for `fmt`, [DefaultFormatter] is used.
func NewLogger(fmt FormatFunc, lvl LogLevel, writers ...io.Writer) *Logger {
	if fmt == nil {
		fmt = DefaultFmt
	}

	return &Logger{
		level:   lvl,
		fmt:     fmt,
		outputs: writers,
		muxs:    make([]sync.Mutex, len(writers)),
	}
}

// NewLoggerOutputs creates a logger that logs at the passed level
// and outputs to the passed outputs, if they are valid. Valid outputs
// are paths (if relative, they will be relative to the executable) and
// "stdout" for stdout. Always returns a logger, but it may not log to
// any outputs if all outputs are invalid.
//
// A [Formatter] can be passed. If `nil` is passed, [DefaultFormatter] is
// used.
func NewLoggerOutputs(level LogLevel, fmt FormatFunc, outputs ...string) *Logger {
	outs := []io.Writer{}
	execPath, execErr := os.Executable()
	if execErr != nil {
		Errorf("logger: Couldn't get executable path (%v), unable to log to relative paths.", execErr.Error())
	}
	execDir := path.Dir(execPath)
	for _, out := range outputs {
		if out == "stdout" {
			outs = append(outs, os.Stdout)
			continue
		}

		var logPath string

		if !path.IsAbs(out) && execErr != nil {
			Errorf("logger: Cannot locate %v, don't know executable path. Will not log to this file.", out)
			continue
		}

		if path.IsAbs(out) {
			logPath = out
		} else {
			logPath = path.Join(execDir, out)
		}

		// Make the directories on the way.
		// If this fails, opening the file will fail too.
		os.MkdirAll(path.Dir(logPath), os.ModePerm)

		logFile, err := os.OpenFile(logPath,
			os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0660)
		if err != nil {
			Errorf("logger: Couldn't open/create log file at %v (%v). Will not log to this file.", out, err.Error())
			continue
		}
		outs = append(outs, logFile)
	}
	return NewLogger(fmt, level, outs...)
}

// Log formats a message and writes to the Logger's outputs if the level is appropriate.
func (logger *Logger) Log(level LogLevel, msg string) {
	// Format message right away in case a timestamp is used.
	s := logger.fmt(msg, level)

	if logger.level <= level {
		for i, out := range logger.outputs {
			logger.muxs[i].Lock()
			fmt.Fprint(out, s)
			logger.muxs[i].Unlock()
		}
	}
}

// Logs a message at Trace level.
func (logger *Logger) Trace(mesg string) {
	logger.Log(LevelTrace, mesg)
}

// Logs a message at Debug level.
func (logger *Logger) Debug(mesg string) {
	logger.Log(LevelDebug, mesg)
}

// Logs a message at Info level.
func (logger *Logger) Info(mesg string) {
	logger.Log(LevelInfo, mesg)
}

// Logs a message at Warn level.
func (logger *Logger) Warn(mesg string) {
	logger.Log(LevelWarning, mesg)
}

// Logs a message at Error level.
func (logger *Logger) Error(mesg string) {
	logger.Log(LevelError, mesg)
}

// Logs a message at Fatal level.
func (logger *Logger) Fatal(mesg string) {
	logger.Log(LevelFatal, mesg)
}

// Logs at Trace level with a format string.
func (logger *Logger) Tracef(format string, a ...any) {
	mesg := fmt.Sprintf(format, a...)
	logger.Trace(mesg)
}

// Logs at Debug level with a format string.
func (logger *Logger) Debugf(format string, a ...any) {
	mesg := fmt.Sprintf(format, a...)
	logger.Debug(mesg)
}

// Logs at Info level with a format string.
func (logger *Logger) Infof(format string, a ...any) {
	mesg := fmt.Sprintf(format, a...)
	logger.Info(mesg)
}

// Logs at Warn level with a format string.
func (logger *Logger) Warnf(format string, a ...any) {
	mesg := fmt.Sprintf(format, a...)
	logger.Warn(mesg)
}

// Logs at Error level with a format string.
func (logger *Logger) Errorf(format string, a ...any) {
	mesg := fmt.Sprintf(format, a...)
	logger.Error(mesg)
}

// Logs at Fatal level with a format string.
func (logger *Logger) Fatalf(format string, a ...any) {
	mesg := fmt.Sprintf(format, a...)
	logger.Fatal(mesg)
}

// Logs in the current logger at Trace level.
func Trace(mesg string) {
	currentLogger.Debug(mesg)
}

// Logs in the current logger at Debug level.
func Debug(mesg string) {
	currentLogger.Debug(mesg)
}

// Logs in the current logger at Info level.
func Info(mesg string) {
	currentLogger.Info(mesg)
}

// Logs in the current logger at Warn level.
func Warn(mesg string) {
	currentLogger.Warn(mesg)
}

// Logs in the current logger at Error level.
func Error(mesg string) {
	currentLogger.Error(mesg)
}

// Logs in the current logger at Fatal level.
func Fatal(mesg string) {
	currentLogger.Fatal(mesg)
}

// Logs in the current logger at Trace level with a format string.
func Tracef(format string, a ...any) {
	currentLogger.Tracef(format, a...)
}

// Logs in the current logger at Debug level with a format string.
func Debugf(format string, a ...any) {
	currentLogger.Debugf(format, a...)
}

// Logs in the current logger at Info level with a format string.
func Infof(format string, a ...any) {
	currentLogger.Infof(format, a...)
}

// Logs in the current logger at Warn level with a format string.
func Warnf(format string, a ...any) {
	currentLogger.Warnf(format, a...)
}

// Logs in the current logger at Error level with a format string.
func Errorf(format string, a ...any) {
	currentLogger.Errorf(format, a...)
}

// Logs in the current logger at Fatal level with a format string.
func Fatalf(format string, a ...any) {
	currentLogger.Fatalf(format, a...)
}
