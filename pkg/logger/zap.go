// Package logger предоставляет структурированное журналирование через zap.
package logger

import (
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger записывает структурированные сообщения с парами ключ-значение.
// With и WithGroup создают производные журналы с дополнительным контекстом.
type Logger interface {
	// Warn записывает предупреждение с дополнительными полями.
	Warn(msg string, keysAndValues ...any)
	// Info записывает информационное сообщение с дополнительными полями.
	Info(msg string, keysAndValues ...any)
	// Error записывает сообщение об ошибке с дополнительными полями.
	Error(msg string, keysAndValues ...any)
	// Debug записывает отладочное сообщение с дополнительными полями.
	Debug(msg string, keysAndValues ...any)
	// With возвращает производный журнал с постоянными полями.
	With(keysAndValues ...any) Logger
	// WithGroup возвращает производный журнал с пространством имён полей.
	WithGroup(name string) Logger
}

// Проверка на уровне компилятора реализует ли ZapLogger интерфейс Logger
var _ Logger = (*ZapLogger)(nil)

// ZapLogger реализует Logger поверх структурированного журнала zap.
type ZapLogger struct {
	logger *zap.SugaredLogger
}

// NewMockLogger создаёт журнал без вывода для тестов и примеров.
func NewMockLogger() *ZapLogger {
	return &ZapLogger{
		logger: zap.NewNop().Sugar(),
	}
}

// NewZapLogger создаёт консольный журнал в stdout с заданным минимальным уровнем.
// Возвращает ошибку, если logLevel не поддерживается zap.
func NewZapLogger(logLevel string) (*ZapLogger, error) {
	logLvl, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		return nil, fmt.Errorf("parsing log level error from zap: %w", err)
	}
	consoleEncoderConfig := zapcore.EncoderConfig{
		TimeKey:        "T",
		LevelKey:       "L",
		NameKey:        "N",
		CallerKey:      "C",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "M",
		StacktraceKey:  "S",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder, // Цветные уровней логирования
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)
	consoleWriter := os.Stdout
	consoleCore := newZapCore(consoleEncoder, consoleWriter, logLvl)

	return &ZapLogger{
		logger: zap.New(consoleCore).Sugar(),
	}, nil

}

func newZapCore(encoder zapcore.Encoder, writer io.Writer, logLevel zapcore.Level) zapcore.Core {
	return zapcore.NewCore(
		encoder,
		zapcore.AddSync(writer),
		logLevel,
	)
}

// Info записывает информационное сообщение с дополнительными полями.
func (zap *ZapLogger) Info(msg string, keysAndValues ...any) {
	zap.logger.Infow(msg, keysAndValues...)
}

// Error записывает сообщение об ошибке с дополнительными полями.
func (zap *ZapLogger) Error(msg string, keysAndValues ...any) {
	zap.logger.Errorw(msg, keysAndValues...)
}

// Debug записывает отладочное сообщение с дополнительными полями.
func (zap *ZapLogger) Debug(msg string, keysAndValues ...any) {
	zap.logger.Debugw(msg, keysAndValues...)
}

// Warn записывает предупреждение с дополнительными полями.
func (zap *ZapLogger) Warn(msg string, keysAndValues ...any) {
	zap.logger.Warnw(msg, keysAndValues...)
}

// With возвращает производный журнал с постоянными полями, сохраняя исходный журнал.
func (z *ZapLogger) With(keysAndValues ...any) Logger {
	return &ZapLogger{
		logger: z.logger.With(keysAndValues...),
	}
}

// WithGroup возвращает производный журнал с пространством имён для последующих полей.
func (z *ZapLogger) WithGroup(name string) Logger {
	return &ZapLogger{
		logger: z.logger.Desugar().
			With(zap.Namespace(name)).
			Sugar(),
	}
}
