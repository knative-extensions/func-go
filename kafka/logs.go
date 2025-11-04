package kafka

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type LogLevel int

const (
	LogDebug LogLevel = iota
	LogInfo
	LogWarn
	LogError
)

func init() {
	initLogging(DefaultLogLevel)
}

func initLogging(level LogLevel) {
	var l zerolog.Level

	switch level {
	case LogDebug:
		l = zerolog.DebugLevel
	case LogInfo:
		l = zerolog.InfoLevel
	case LogWarn:
		l = zerolog.WarnLevel
	case LogError:
		l = zerolog.ErrorLevel
	default:
		l = zerolog.DebugLevel
	}

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).Level(l)
}
