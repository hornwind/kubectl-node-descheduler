package logging

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ValidateLogLevel validates that the provided log level is valid
func ValidateLogLevel(level string) error {
	var zapLevel zapcore.Level
	if level == "" {
		return fmt.Errorf("log level is empty")
	}
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		return fmt.Errorf("invalid log level %q: %w", level, err)
	}
	return nil
}

type Logger struct {
	*zap.SugaredLogger
}

var defaultLogger *Logger

func GetLogger() *Logger {
	return defaultLogger
}

func (l *Logger) GetLoggerWithField(k string, v interface{}) *Logger {
	return &Logger{l.With(k, v)}
}

func (l *Logger) GetLoggerWithFields(fields map[string]any) *Logger {
	// Convert map to key-value pairs for zap
	args := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return &Logger{l.With(args...)}
}

func (l *Logger) SetLogLevel(level string) error {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		return err
	}

	// Create new logger configuration with the new level
	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapLevel)
	config.Encoding = "console"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.LevelKey = zapcore.OmitKey
	config.DisableStacktrace = true
	config.DisableCaller = true

	// Create new logger
	newLogger, err := config.Build(
		zap.AddCallerSkip(1),
	)
	if err != nil {
		return err
	}

	// Replace the existing logger
	l.SugaredLogger = newLogger.Sugar()
	return nil
}

func init() {
	// Create the default production configuration
	config := zap.NewProductionConfig()
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.DisableStacktrace = true
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	config.Encoding = "console"
	config.DisableCaller = true

	// Build the logger
	logger, err := config.Build(
		zap.AddCallerSkip(1),
	)
	if err != nil {
		panic(err)
	}

	// Create and set the default logger
	defaultLogger = &Logger{logger.Sugar()}
}
