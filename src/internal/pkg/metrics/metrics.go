package metrics

import (
	"fmt"
	"github.com/rcrowley/go-metrics"
	"sync"
	"time"
)

// Структура для всех метрик приложения
type AppMetrics struct {
	// Счётчики (простое увеличение)
	//TotalRequests   metrics.Counter
	//ErrorRequests   metrics.Counter
	//SuccessRequests metrics.Counter
	//
	//// Метры (частота событий по времени)
	//RequestRate metrics.Meter // Общая частота запросов
	//ErrorRate   metrics.Meter // Частота ошибок

	// Время старта приложения
	StartTime time.Time

	// Карта метрик по типам запросов
	RequestTypes map[string]*RequestTypeMetrics

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
		//// Инициализация счётчиков
		//TotalRequests:   metrics.NewCounter(),
		//ErrorRequests:   metrics.NewCounter(),
		//SuccessRequests: metrics.NewCounter(),
		//
		//// Инициализация метров
		//RequestRate: metrics.NewMeter(),
		//ErrorRate:   metrics.NewMeter(),

		RequestTypes: make(map[string]*RequestTypeMetrics),
		// Устанавливаем время старта
		StartTime: time.Now(),
	}
}

// Метод для регистрации всех метрик в go-metrics registry
func (m *AppMetrics) Start() {
	//metrics.Register("app.requests.total", m.TotalRequests)
	//metrics.Register("app.requests.errors", m.ErrorRequests)
	//metrics.Register("app.requests.success", m.SuccessRequests)
	//
	//metrics.Register("app.rate.requests", m.RequestRate)
	//metrics.Register("app.rate.errors", m.ErrorRate)

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

func (m *AppMetrics) IncrementSuccessRequest(requestType string) {
	metric := m.GetRequestTypeMetricsSafe(requestType)
	metric.IncrementSuccessRequest()
}
func (m *AppMetrics) IncrementErrorRequest(requestType string) {
	metric := m.GetRequestTypeMetricsSafe(requestType)
	metric.IncrementErrorRequest()
}
