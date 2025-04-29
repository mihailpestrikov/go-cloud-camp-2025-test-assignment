package logger

import (
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go-cloud-camp-2025-test-assignment/config"
	"io"
	"os"
	"strings"
	"time"
)

func Setup(cfg config.LoggerConfig) {
	level, err := zerolog.ParseLevel(strings.ToLower(cfg.Level))
	if err != nil {
		level = zerolog.DebugLevel
	}
	zerolog.SetGlobalLevel(level)

	var output io.Writer = os.Stdout

	if cfg.Output == "file" && cfg.FilePath != "" {
		file, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err == nil {
			output = file
		}
	}

	var writer = output
	if cfg.Format == "console" {
		writer = zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
	}

	logger := zerolog.New(writer).With().Timestamp()

	log.Logger = logger.Logger()

	log.Info().
		Str("level", level.String()).
		Str("format", cfg.Format).
		Str("output", cfg.Output).
		Msg("Logger initialized")
}
