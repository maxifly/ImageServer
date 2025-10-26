package metrics

import (
	"fmt"
	"github.com/rcrowley/go-metrics"
	"log"
	"log/slog"
	"time"
)

type SlogAdapter struct {
	logger *slog.Logger
	prefix string
}

func NewSlogAdapter(logger *slog.Logger, prefix string) *SlogAdapter {
	return &SlogAdapter{
		logger: logger,
		prefix: prefix,
	}
}

func (a *SlogAdapter) Write(p []byte) (n int, err error) {
	// Убираем переносы строк из вывода go-metrics
	msg := string(p)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}

	// Логируем каждую строку как отдельное сообщение
	lines := splitLines(msg)
	for _, line := range lines {
		if line != "" {
			a.logger.Info(fmt.Sprintf("%s%s", a.prefix, line))
		}
	}

	return len(p), nil
}

// Вспомогательная функция для разделения строк
func splitLines(s string) []string {
	if s == "" {
		return nil
	}

	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			if i > start {
				lines = append(lines, s[start:i])
			}
			start = i + 1
		}
	}

	if start < len(s) {
		lines = append(lines, s[start:])
	}

	return lines
}

func StartMetricsLogging(logger *slog.Logger, interval time.Duration) {
	adapter := NewSlogAdapter(logger, "metrics: ")

	go metrics.Log(metrics.DefaultRegistry, interval,
		log.New(adapter, "", log.LstdFlags))
}
