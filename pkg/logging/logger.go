package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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

func (l *Logger) GetLoggerWithFields(fields map[string]interface{}) *Logger {
	return &Logger{l.With(fields)}
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
