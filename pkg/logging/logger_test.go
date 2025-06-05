package logging

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestMain(m *testing.M) {
	// Create test logger configuration
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

	// Run tests
	code := m.Run()
	os.Exit(code)
}

func TestValidateLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		level   string
		wantErr bool
	}{
		{
			name:    "debug level",
			level:   "debug",
			wantErr: false,
		},
		{
			name:    "info level",
			level:   "info",
			wantErr: false,
		},
		{
			name:    "warn level",
			level:   "warn",
			wantErr: false,
		},
		{
			name:    "error level",
			level:   "error",
			wantErr: false,
		},
		{
			name:    "invalid level",
			level:   "invalid",
			wantErr: true,
		},
		{
			name:    "empty level",
			level:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLogLevel(tt.level)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestLogger_SetLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		level     string
		wantErr   bool
		checkFunc func(*testing.T, *Logger)
	}{
		{
			name:    "set debug level",
			level:   "debug",
			wantErr: false,
			checkFunc: func(t *testing.T, l *Logger) {
				assert.True(t, l.Desugar().Core().Enabled(zapcore.DebugLevel))
			},
		},
		{
			name:    "set info level",
			level:   "info",
			wantErr: false,
			checkFunc: func(t *testing.T, l *Logger) {
				assert.False(t, l.Desugar().Core().Enabled(zapcore.DebugLevel))
				assert.True(t, l.Desugar().Core().Enabled(zapcore.InfoLevel))
			},
		},
		{
			name:    "set warn level",
			level:   "warn",
			wantErr: false,
			checkFunc: func(t *testing.T, l *Logger) {
				assert.False(t, l.Desugar().Core().Enabled(zapcore.InfoLevel))
				assert.True(t, l.Desugar().Core().Enabled(zapcore.WarnLevel))
			},
		},
		{
			name:    "set error level",
			level:   "error",
			wantErr: false,
			checkFunc: func(t *testing.T, l *Logger) {
				assert.False(t, l.Desugar().Core().Enabled(zapcore.WarnLevel))
				assert.True(t, l.Desugar().Core().Enabled(zapcore.ErrorLevel))
			},
		},
		{
			name:    "invalid level",
			level:   "invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := GetLogger()
			err := logger.SetLogLevel(tt.level)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			if tt.checkFunc != nil {
				tt.checkFunc(t, logger)
			}
		})
	}
}

func TestLogger_GetLoggerWithField(t *testing.T) {
	logger := GetLogger()
	newLogger := logger.GetLoggerWithField("key", "value")
	assert.NotNil(t, newLogger)
	assert.NotEqual(t, logger, newLogger)
}

func TestLogger_GetLoggerWithFields(t *testing.T) {
	logger := GetLogger()
	assert.NotNil(t, logger, "Initial logger should not be nil")

	fields := map[string]any{
		"key1": "value1",
		"key2": 42,
		"key3": true,
	}

	newLogger := logger.GetLoggerWithFields(fields)

	assert.NotNil(t, newLogger, "New logger should not be nil")
	assert.NotEqual(t, logger, newLogger, "New logger should be different from original logger")

	assert.NotEqual(t, logger.SugaredLogger, newLogger.SugaredLogger, "Underlying loggers should be different")
}
