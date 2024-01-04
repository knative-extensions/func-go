package common

import (
	"github.com/rs/zerolog"
)

const (
	DefaultLogLevel = LogDebug
)

func init() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	SetLogLevel(DefaultLogLevel)
}

type logLevel zerolog.Level

const (
	LogDebug    = logLevel(zerolog.DebugLevel)
	LogInfo     = logLevel(zerolog.InfoLevel)
	LogWarn     = logLevel(zerolog.WarnLevel)
	LogDisabled = logLevel(zerolog.Disabled)
)

// SetLogLevel to LogDebug, LogInfo, LogWarn, or LogDisabled
// Errors are always returned as values.
func SetLogLevel(l logLevel) {
	zerolog.SetGlobalLevel(zerolog.Level(l))
}
