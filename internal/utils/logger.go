package utils

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger

func NewLogger(appName string) *zap.Logger {
	isLocal := os.Getenv("NODE_ENV") != "production"
	hostName, _ := os.Hostname()

	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"stdout"}

	// log level
	if isLocal {
		config.Level.SetLevel(zap.DebugLevel)
	} else {
		config.Level.SetLevel(zap.InfoLevel)
	}

	// caller
	config.EncoderConfig.CallerKey = "context"

	// error stack
	config.EncoderConfig.StacktraceKey = "stack"
	config.DisableStacktrace = false

	// encode level as number like pino does. no trace level though
	config.EncoderConfig.EncodeLevel = func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
		num := (int8(l) + 3) * 10
		if num > 50 {
			enc.AppendInt8(50)
		} else {
			enc.AppendInt8(num)
		}
	}

	// timing
	config.EncoderConfig.TimeKey = "time"
	config.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		nanos := t.UnixNano()
		millis := float64(nanos) / float64(time.Millisecond)
		enc.AppendInt64(int64(millis))
	}

	logger = zap.Must(config.Build(zap.Fields(
		zap.Int("pid", os.Getpid()),
		zap.String("hostname", hostName),
		zap.String("name", appName),
	)))

	return logger
}
