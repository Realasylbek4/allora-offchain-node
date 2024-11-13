package main

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func initLogger() {
	// Set time format based on environment variable
	timeFormat := os.Getenv("LOG_TIME_FORMAT")
	switch strings.ToLower(timeFormat) {
	case "unix":
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	case "unixms":
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	case "unixmicro":
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro
	case "iso8601":
		zerolog.TimeFieldFormat = time.RFC3339
	default:
		// If not set or invalid, default to RFC3339 (zerolog default)
		zerolog.TimeFieldFormat = time.RFC3339
	}

	// Get log level from environment variable
	logLevel := strings.ToLower(os.Getenv("LOG_LEVEL"))

	switch logLevel {
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	case "fatal":
		zerolog.SetGlobalLevel(zerolog.FatalLevel)
	case "panic":
		zerolog.SetGlobalLevel(zerolog.PanicLevel)
	default:
		// If not set or invalid, default to Info
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	log.Info().Msg("Logging initialized")
	log.Info().Msg("Log level set to: " + zerolog.GlobalLevel().String())
	log.Info().Msg("Time format set to: " + zerolog.TimeFieldFormat)
	log.Info().Msg("================================================")
}
