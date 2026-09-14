package logger

import (
	"sync/atomic"
)

var (
	std atomic.Pointer[Logger]
)

func init() {
	std.Store(New())
}

func Default() *Logger             { return std.Load() }
func SetDefault(l *Logger) *Logger { return std.Swap(l) }
func SetVerbose(v bool)            { std.Load().SetVerbose(v) }
func IsVerbose() bool              { return std.Load().IsVerbose() }
func Info(msg string)              { std.Load().Info(msg) }
func Infof(f string, a ...any)     { std.Load().Infof(f, a...) }
func Error(msg string)             { std.Load().Error(msg) }
func Errorf(f string, a ...any)    { std.Load().Errorf(f, a...) }
func Success(msg string)           { std.Load().Success(msg) }
func Successf(f string, a ...any)  { std.Load().Successf(f, a...) }
func Warning(msg string)           { std.Load().Warning(msg) }
func Warningf(f string, a ...any)  { std.Load().Warningf(f, a...) }
func Dim(msg string)               { std.Load().Dim(msg) }
func Dimf(f string, a ...any)      { std.Load().Dimf(f, a...) }
func Debug(msg string)             { std.Load().Debug(msg) }
func Debugf(f string, a ...any)    { std.Load().Debugf(f, a...) }
func LogMultilineYaml(v any)       { std.Load().LogMultilineYaml(v) }
