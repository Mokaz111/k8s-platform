package logger

import (
	"os"
	"path/filepath"

	"github.com/k8s-platform/console/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Logger 封装 zap.SugaredLogger，提供便捷的结构化日志方法
type Logger struct {
	*zap.SugaredLogger
	zap *zap.Logger
}

// New 按配置初始化全局日志
func New(cfg config.LogConfig) *Logger {
	// 日志级别
	level := parseLevel(cfg.Level)

	// 编码器
	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderCfg.EncodeDuration = zapcore.MillisDurationEncoder

	var encoder zapcore.Encoder
	if cfg.Format == "console" {
		encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}

	// 多输出：stdout + 可选 error 文件
	cores := make([]zapcore.Core, 0, 2)
	stdoutWS := zapcore.AddSync(os.Stdout)
	cores = append(cores, zapcore.NewCore(encoder, stdoutWS, level))

	if cfg.ErrorFile != "" {
		_ = os.MkdirAll(filepath.Dir(cfg.ErrorFile), 0755)
		fileWS := zapcore.AddSync(&lumberjack.Logger{
			Filename:   cfg.ErrorFile,
			MaxSize:    cfg.MaxSize,
			MaxAge:     cfg.MaxAge,
			MaxBackups: cfg.MaxBackups,
			Compress:   cfg.Compress,
		})
		// 错误文件只记录 error 及以上
		errLevel := zap.LevelEnablerFunc(func(l zapcore.Level) bool {
			return l >= zap.ErrorLevel
		})
		cores = append(cores, zapcore.NewCore(encoder, fileWS, errLevel))
	}

	core := zapcore.NewTee(cores...)

	z := zap.New(core,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zap.ErrorLevel),
	)

	return &Logger{SugaredLogger: z.Sugar(), zap: z}
}

// With 返回带上下文字段的新 Logger（KV 对，用于 trace_id/user_id 等透传）
func (l *Logger) With(keysAndValues ...interface{}) *Logger {
	return &Logger{SugaredLogger: l.SugaredLogger.With(keysAndValues...), zap: l.zap}
}

// Sync 刷新缓冲区（defer 调用）
func (l *Logger) Sync() {
	_ = l.zap.Sync()
}

// Zap 返回底层 *zap.Logger（用于接入第三方库）
func (l *Logger) Zap() *zap.Logger { return l.zap }

func parseLevel(l string) zapcore.Level {
	switch l {
	case "debug":
		return zap.DebugLevel
	case "info":
		return zap.InfoLevel
	case "warn", "warning":
		return zap.WarnLevel
	case "error":
		return zap.ErrorLevel
	case "panic":
		return zap.PanicLevel
	case "fatal":
		return zap.FatalLevel
	default:
		return zap.InfoLevel
	}
}
