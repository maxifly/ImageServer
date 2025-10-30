package metrics

import (
	"fmt"
	"github.com/rcrowley/go-metrics"
	"sync"
	"time"
)

// Структура для всех метрик приложения
type AppMetrics struct {

	// Время старта приложения
	StartTime time.Time

	// Карта метрик по типам запросов
	RequestTypes  map[string]*RequestTypeMetrics
	DailyCounters map[string]*DailyCounter
	cleanupPeriod time.Duration
	ttl           time.Duration
	ticker        *time.Ticker

	// Мьютекс для безопасной работы с map
	mu sync.RWMutex
}

type RequestTypeMetrics struct {
	Total       metrics.Counter // Общее количество запросов
	Success     metrics.Counter // Успешные запросы
	Errors      metrics.Counter // Ошибки
	TotalRate   metrics.Meter   // Частота запросов/сек
	ErrorRate   metrics.Meter   // Частота запросов/сек
	SuccessRate metrics.Meter   // Частота запросов/сек
}

type DailyCounter struct {
	Counter      metrics.Counter
	EvictDate    time.Time
	RegistryName string
}

func (metric *RequestTypeMetrics) IncrementSuccessRequest() {
	metric.incrementRequest(false)
}
func (metric *RequestTypeMetrics) IncrementErrorRequest() {
	metric.incrementRequest(true)
}

func (metric *RequestTypeMetrics) incrementRequest(isError bool) {
	metric.Total.Inc(1)
	metric.TotalRate.Mark(1)
	if isError {
		metric.Errors.Inc(1)
		metric.ErrorRate.Mark(1)

	} else {
		metric.Success.Inc(1)
		metric.SuccessRate.Mark(1)
	}
}

// Конструктор для создания метрик
func NewAppMetrics() *AppMetrics {
	return &AppMetrics{
		RequestTypes:  make(map[string]*RequestTypeMetrics),
		DailyCounters: make(map[string]*DailyCounter),
		// Устанавливаем время старта
		StartTime:     time.Now(),
		ttl:           time.Duration(48) * time.Hour,
		cleanupPeriod: time.Duration(24) * time.Hour,
	}

}

// Start Метод для старта
func (m *AppMetrics) Start() {
	m.startDailyCleanupRoutine()
}

func (m *AppMetrics) GetRequestTypeMetricsSafe(requestType string) *RequestTypeMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.RequestTypes[requestType]; ok {
		return existing
	}

	// Создаем и регистрируем
	metric := &RequestTypeMetrics{
		Total:       metrics.NewCounter(),
		Success:     metrics.NewCounter(),
		Errors:      metrics.NewCounter(),
		TotalRate:   metrics.NewMeter(),
		ErrorRate:   metrics.NewMeter(),
		SuccessRate: metrics.NewMeter(),
	}

	// Регистрируем с уникальными именами
	typeName := fmt.Sprintf("app.requests.%s", requestType)
	metrics.GetOrRegister(fmt.Sprintf("%s.total", typeName), metric.Total)
	metrics.GetOrRegister(fmt.Sprintf("%s.success", typeName), metric.Success)
	metrics.GetOrRegister(fmt.Sprintf("%s.errors", typeName), metric.Errors)
	metrics.GetOrRegister(fmt.Sprintf("%s.total_rate", typeName), metric.TotalRate)
	metrics.GetOrRegister(fmt.Sprintf("%s.error_rate", typeName), metric.ErrorRate)
	metrics.GetOrRegister(fmt.Sprintf("%s.success_rate", typeName), metric.SuccessRate)

	m.RequestTypes[requestType] = metric
	return metric
}

func (m *AppMetrics) GetDailyMetricSafe(metricTime time.Time, metricType string) *DailyCounter {
	m.mu.Lock()
	defer m.mu.Unlock()

	today := metricTime.Format("2006-01-02")
	key := fmt.Sprintf("%s_%s", metricType, today)

	if existing, ok := m.DailyCounters[key]; ok {
		return existing
	}

	// Создаем и регистрируем
	metric := &DailyCounter{
		Counter:   metrics.NewCounter(),
		EvictDate: truncateToDay(metricTime.Add(m.ttl)),
	}

	// Регистрируем с уникальными именами
	metric.RegistryName = fmt.Sprintf("app.daily.%s", key)
	metrics.GetOrRegister(metric.RegistryName, metric)

	m.DailyCounters[key] = metric
	return metric
}

func (m *AppMetrics) IncrementSuccessRequest(requestType string) {
	metric := m.GetRequestTypeMetricsSafe(requestType)
	metric.IncrementSuccessRequest()
}
func (m *AppMetrics) IncrementErrorRequest(requestType string) {
	metric := m.GetRequestTypeMetricsSafe(requestType)
	metric.IncrementErrorRequest()
}
func (m *AppMetrics) IncrementDaily(mType string) {
	metric := m.GetDailyMetricSafe(time.Now(), mType)
	metric.Counter.Inc(1)
}

func (m *AppMetrics) cleanDaily() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()

	oldKeys := make(map[string]string)

	for key, metric := range m.DailyCounters {
		if metric.EvictDate.Before(now) {
			oldKeys[key] = metric.RegistryName
		}
	}

	for key, registryName := range oldKeys {
		metrics.Unregister(registryName)
		delete(m.DailyCounters, key)
	}
}

func (m *AppMetrics) startDailyCleanupRoutine() {
	m.ticker = time.NewTicker(m.cleanupPeriod)
	go func() {
		for range m.ticker.C {
			m.cleanDaily()
		}
	}()
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
