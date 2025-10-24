package opermanager

import (
	"fmt"
	"sync"
	"time"
)

type TimeRange struct {
	Start string `yaml:"start_time"` // Формат: "HH:MM" или "HH:MM:SS"
	End   string `yaml:"end_time"`   // Формат: "HH:MM" или "HH:MM:SS"

	// Кэшированные значения
	mu        sync.RWMutex
	startTime time.Time
	endTime   time.Time
	startDay  time.Time // Дата последнего обновления кэша
}

func (tr *TimeRange) String() string {

	// Формируем описание периода
	periodDesc := fmt.Sprintf("Период %s - %s", tr.Start, tr.End)

	return fmt.Sprintf("%s", periodDesc)
}

func (tr *TimeRange) IsWithinRangeInclusive(now time.Time) (bool, error) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	// Проверяем, нужно ли обновить кэш (дата изменилась или значения не инициализированы)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	if tr.startDay.IsZero() || !tr.startDay.Equal(today) {
		if err := tr.updateCache(); err != nil {
			return false, err
		}
		tr.startDay = today
	}

	currentTime := time.Date(now.Year(), now.Month(), now.Day(),
		now.Hour(), now.Minute(), now.Second(), 0, now.Location())

	// Проверка с включенными границами
	if tr.startTime.After(tr.endTime) {
		// Период через полночь
		return !currentTime.Before(tr.startTime) || !currentTime.After(tr.endTime), nil
	}

	return !currentTime.Before(tr.startTime) && !currentTime.After(tr.endTime), nil
}

func (tr *TimeRange) updateCache() error {
	// Парсим время только один раз при обновлении кэша
	start, err := tr.parseTime(tr.Start)
	if err != nil {
		return err
	}

	end, err := tr.parseTime(tr.End)
	if err != nil {
		return err
	}

	// Устанавливаем на сегодняшнюю дату
	today := time.Now()
	tr.startTime = time.Date(today.Year(), today.Month(), today.Day(),
		start.Hour(), start.Minute(), start.Second(), 0, today.Location())
	tr.endTime = time.Date(today.Year(), today.Month(), today.Day(),
		end.Hour(), end.Minute(), end.Second(), 0, today.Location())

	return nil
}

func (tr *TimeRange) parseTime(timeStr string) (time.Time, error) {
	// Пробуем разные форматы
	formats := []string{"15:04:05", "15:04"}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("неверный формат времени: %s", timeStr)
}
