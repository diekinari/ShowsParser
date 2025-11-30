package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var sugar *zap.SugaredLogger

// Init initializes the global logger
func Init(level string) error {
	logLevel := zap.InfoLevel
	switch strings.ToLower(level) {
	case "debug":
		logLevel = zap.DebugLevel
	case "info":
		logLevel = zap.InfoLevel
	case "warn":
		logLevel = zap.WarnLevel
	case "error":
		logLevel = zap.ErrorLevel
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder // Colored levels
	encoderConfig.TimeKey = "timestamp"
	encoderConfig.LevelKey = "level"
	encoderConfig.MessageKey = "message"
	encoderConfig.CallerKey = "caller"
	encoderConfig.StacktraceKey = "stacktrace"

	// Use ConsoleEncoder for "more presentable" human-readable logs,
	// or JSONEncoder for structured logs.
	// The user asked for "presentable", so console encoder with colors is usually better for reading in terminal.
	encoder := zapcore.NewConsoleEncoder(encoderConfig)

	core := zapcore.NewCore(
		encoder,
		zapcore.AddSync(os.Stdout),
		logLevel,
	)

	// Add caller skip to correctly identify the caller when wrapping (though we are returning the logger)
	logger := zap.New(core, zap.AddCaller())

	sugar = logger.Sugar()
	return nil
}

// Get returns the global sugared logger instance
func Get() *zap.SugaredLogger {
	if sugar == nil {
		// Fallback if Init wasn't called
		Init("info")
	}
	return sugar
}
