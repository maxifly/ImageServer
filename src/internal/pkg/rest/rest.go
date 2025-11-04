package rest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"html/template"
	"imgserver/internal/pkg/helpers"
	"imgserver/internal/pkg/metrics"
	"imgserver/internal/pkg/opermanager"
	"imgserver/internal/pkg/promptmanager"
	"imgserver/internal/pkg/ydart"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	METRIC_ALL_WEB          = "WEB_ALL"
	METRIC_OPERATION_START  = "OPERATION_START"
	METRIC_OPERATION_STATUS = "OPERATION_STATUS"
	METRIC_NEW_PROMPT       = "NEW_PROMPT"
	METRIC_IMAGE_GET        = "IMAGE_GET"
)

// Шаблон для веб-страницы
var indexTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>Image Server Status</title>
    <script>
        function sendRequest() {
            fetch('/internal_function', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({})
            })
            .then(response => response.json())
            .then(data => {
                if(data.success){
                    alert('Function executed successfully');
                } else {
                    alert('Error: ' + data.error);
                }
            })
            .catch((error) => {
                console.error('Error:', error);
            });
        }
    </script>
</head>
<body>
    <h1>Image Server Status</h1>
    <p>Total Requests: {{.TotalRequests}}</p>
    <p>Total error requests: {{.TotalRequestsError}}</p>
    <p>Total requests success rate (req per hour): {{.TotalRequestsSuccessRate}}</p>
    <p>Total requests error rate (req per hour): {{.TotalRequestsErrorRate}}</p>
    </br>
    <p>Images Sent: {{.ImagesSentTotal}}</p>
    <p>Images error sent: {{.ImagesSentError}}</p>
    <p>Images sent success rate (req per hour): {{.ImagesSentSuccessRate}}</p>
    <p>Images sent error rate (req per hour): {{.ImagesSentErrorRate}}</p>
    </br>
    <p>Yandex art yesterday success: {{.YandexYesterday}}</p>
    <p>Yandex art today success: {{.YandexToday}}</p>

    </br>
    <p>Yandex art success: {{.YandexTotal}}</p>
    <p>Yandex art error: {{.YandexError}}</p>
    <p>Yandex art success rate (req per hour): {{.YandexSuccessRate}}</p>
    <p>Yandex art error rate (req per hour): {{.YandexErrorRate}}</p>


    <button onclick="sendRequest()">Execute Internal Function</button>
</body>
</html>
`

type Rest struct {
	logger        *slog.Logger
	router        *mux.Router
	operMng       *opermanager.OperMngr
	port          string
	promptManager *promptmanager.PromptManager
	metrics       *metrics.AppMetrics
}

func NewRest(port string,
	logger *slog.Logger,
	operMng *opermanager.OperMngr,
	promptManager *promptmanager.PromptManager,
	metrics *metrics.AppMetrics,
) (*Rest, error) {

	router := mux.NewRouter()

	restObj := Rest{port: port,
		router:        router,
		logger:        logger,
		operMng:       operMng,
		promptManager: promptManager,
		metrics:       metrics,
	}

	router.HandleFunc("/", restObj.handleIndex).Methods("GET")
	router.HandleFunc("/operation/start", restObj.handleStartOperation).Methods("POST")
	router.HandleFunc("/operation/status/{operationId}", restObj.handleGetOperationStatus).Methods("GET")
	router.HandleFunc("/operation/result/{operationId}", restObj.handleGetImage).Methods("GET")
	router.HandleFunc("/prompt/add", restObj.handleNewPrompt).Methods("POST")

	logger.Error("(It is not error!!!) Run WEB-Server on https://127.0.0.1", "port", port)

	return &restObj, nil

}

func (rest *Rest) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Парсим шаблон
	tmpl, err := template.New("index").Parse(indexTemplate)
	if err != nil {
		rest.logger.Warn("Error parsing template %v", err)
		http.Error(w, "Error parsing template", http.StatusInternalServerError)
		return
	}

	// Выполняем шаблон

	ydArtMetric := rest.metrics.GetRequestTypeMetricsSafe(opermanager.METRIC_TEMPLATE_OPERATION_START + ydart.ProviderCode)

	err = tmpl.Execute(w, StatusResponse{
		TotalRequests:            rest.metrics.GetRequestTypeMetricsSafe(METRIC_ALL_WEB).Total.Count(),
		TotalRequestsError:       rest.metrics.GetRequestTypeMetricsSafe(METRIC_ALL_WEB).Errors.Count(),
		TotalRequestsSuccessRate: helpers.RoundToTwoDecimals(rest.metrics.GetRequestTypeMetricsSafe(METRIC_ALL_WEB).SuccessRate.Rate15() * 3600.),
		TotalRequestsErrorRate:   helpers.RoundToTwoDecimals(rest.metrics.GetRequestTypeMetricsSafe(METRIC_ALL_WEB).ErrorRate.Rate15() * 3600.),
		ImagesSentTotal:          rest.metrics.GetRequestTypeMetricsSafe(METRIC_IMAGE_GET).Total.Count(),
		ImagesSentError:          rest.metrics.GetRequestTypeMetricsSafe(METRIC_IMAGE_GET).Errors.Count(),
		ImagesSentSuccessRate:    helpers.RoundToTwoDecimals(rest.metrics.GetRequestTypeMetricsSafe(METRIC_IMAGE_GET).SuccessRate.Rate15() * 3600.),
		ImagesSentErrorRate:      helpers.RoundToTwoDecimals(rest.metrics.GetRequestTypeMetricsSafe(METRIC_IMAGE_GET).ErrorRate.Rate15() * 3600.),

		YandexTotal:       ydArtMetric.Total.Count(),
		YandexError:       ydArtMetric.Errors.Count(),
		YandexSuccessRate: helpers.RoundToTwoDecimals(ydArtMetric.SuccessRate.Rate15() * 3600.),
		YandexErrorRate:   helpers.RoundToTwoDecimals(ydArtMetric.ErrorRate.Rate15() * 3600.),

		YandexYesterday: rest.metrics.GetDailyMetricSafe(time.Now().Add(-time.Duration(24)*time.Hour), opermanager.METRIC_TEMPLATE_OPERATION_START+ydart.ProviderCode).Counter.Count(),
		YandexToday:     rest.metrics.GetDailyMetricSafe(time.Now(), opermanager.METRIC_TEMPLATE_OPERATION_START+ydart.ProviderCode).Counter.Count(),
	})

	if err != nil {
		http.Error(w, "Error executing template", http.StatusInternalServerError)
		return
	}
}

func (rest *Rest) handleGetImage(w http.ResponseWriter, r *http.Request) {
	rest.logger.Debug("Handling GET image")
	vars := mux.Vars(r)
	var errorAttrs ErrorAttributes

	operationId, ok := vars["operationId"]
	if !ok {
		errorAttrs.Code = "BadRequest"
		errorAttrs.Message = "operationId is missing in parameters"
		var errorResp ErrorResponse
		errorResp.Error = errorAttrs
		sendJSONResponse(w, http.StatusBadRequest, errorResp)
		rest.logger.Error(errorAttrs.Message)
		rest.incrRequestMetric(METRIC_IMAGE_GET, true)
		return
	}

	rest.logger.Debug("Operation id " + operationId)

	status, err := rest.operMng.GetOperationStatus(operationId)

	if err != nil {
		errorAttrs.Code = "InternalError"
		errorAttrs.Message = "Can not get operation status"
		errorAttrs.DevMessage = err.Error()
		errorResp := ErrorResponse{errorAttrs}
		sendJSONResponse(w, http.StatusUnprocessableEntity, errorResp)
		rest.logger.Error(errorAttrs.Message, slog.String("error", errorAttrs.DevMessage))
		rest.incrRequestMetric(METRIC_IMAGE_GET, true)
		return
	}
	var imageResponse = ImageResponse{Id: operationId, Status: status.Status, Error: nil}

	if len(status.Error) > 0 {
		errorAttrs.Code = "operationError"
		errorAttrs.Message = "operation have error status"
		errorAttrs.DevMessage = status.Error
		imageResponse.Error = &errorAttrs
	}
	if status.Status == opermanager.StatusError {
		sendJSONResponse(w, http.StatusUnprocessableEntity, imageResponse)
		rest.incrRequestMetric(METRIC_IMAGE_GET, true)
		return
	}

	if status.Status != opermanager.StatusDone {
		sendJSONResponse(w, http.StatusOK, imageResponse)
		rest.incrRequestMetric(METRIC_IMAGE_GET, false)
		return
	}

	fileName, err := rest.operMng.GetFileName(operationId)
	if err != nil {
		errorAttrs.Code = "InternalError"
		errorAttrs.Message = "Can not get filename"
		errorAttrs.DevMessage = err.Error()
		errorResp := ErrorResponse{errorAttrs}
		sendJSONResponse(w, http.StatusUnprocessableEntity, errorResp)
		rest.logger.Error(errorAttrs.Message, slog.String("error", errorAttrs.DevMessage))
		rest.incrRequestMetric(METRIC_IMAGE_GET, true)
		return
	}

	rest.logger.Debug("Send file", "operatioId", operationId, "filename", fileName)

	data, err := os.ReadFile(fileName)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			http.Error(w, "Error reading file", http.StatusInternalServerError)
		}
		rest.incrRequestMetric(METRIC_IMAGE_GET, true)
		return
	}

	// Кодируем данные в base64
	encoded := base64.StdEncoding.EncodeToString(data)

	imageResult := ImageResultResponse{Image: encoded}
	imageResponse.Result = imageResult

	//sendJSONResponse(w, http.StatusOK, imageResponse)

	// Кодируем структуру в JSON
	jsonData, err := json.Marshal(imageResponse)
	if err != nil {
		http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
		rest.incrRequestMetric(METRIC_IMAGE_GET, true)
		return
	}

	// Получаем размер чанка из параметров запроса, по умолчанию 1024
	chunkSizeParam := r.URL.Query().Get("chunk_size")
	chunkSize := 512
	if chunkSizeParam != "" {
		cs, err := strconv.Atoi(chunkSizeParam)
		if err == nil && cs > 0 {
			chunkSize = cs
		}
	}

	// Отправляем данные чанками
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		rest.incrRequestMetric(METRIC_IMAGE_GET, true)
		return
	}

	// Устанавливаем заголовки для потоковой передачи
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Transfer-Encoding", "chunked")

	// Отправляем данные
	for i := 0; i < len(jsonData); i += chunkSize {
		end := i + chunkSize
		if end > len(jsonData) {
			end = len(jsonData)
		}
		chunk := jsonData[i:end]
		w.Write(chunk)
		flusher.Flush()
	}
	rest.incrRequestMetric(METRIC_IMAGE_GET, false)
}

// Функция для обработки POST-запросов к /operation/start
func (rest *Rest) handleStartOperation(w http.ResponseWriter, r *http.Request) {

	// Создаем ответ
	var startResp StartResponse
	var errorAttrs ErrorAttributes

	w.Header().Set("Content-Type", "application/json")

	// Читаем тело запроса
	var startReq StartRequest
	err := json.NewDecoder(r.Body).Decode(&startReq)
	if err != nil {
		errorAttrs.Code = "BadRequest"
		errorAttrs.Message = "Error parsing JSON request"
		startResp.Error = errorAttrs
		sendJSONResponse(w, http.StatusBadRequest, startResp)
		rest.logger.Error(errorAttrs.Message)
		rest.incrRequestMetric(METRIC_OPERATION_START, true)
		return
	}

	// Проверяем тип операции
	if startReq.Type != "auto" && startReq.Type != "ydart" && startReq.Type != "old" {
		http.Error(w, "Invalid operation type", http.StatusBadRequest)
		rest.incrRequestMetric(METRIC_OPERATION_START, true)
		return
	}

	operationId, err := rest.operMng.StartOperation(startReq.Type, startReq.Prompt)
	if err != nil {
		errorAttrs.Code = "StartError"
		errorAttrs.Message = "Can not start operation"
		errorAttrs.DevMessage = err.Error()
		startResp.Error = errorAttrs
		sendJSONResponse(w, http.StatusBadRequest, startResp)
		rest.logger.Error(errorAttrs.Message, slog.String("error", errorAttrs.DevMessage))
		rest.incrRequestMetric(METRIC_OPERATION_START, true)
		return
	}

	startResp.ID = operationId
	startResp.Status = opermanager.StatusPending
	rest.incrRequestMetric(METRIC_OPERATION_START, false)
	sendJSONResponse(w, http.StatusOK, startResp)
}

// Функция для обработки POST-запросов к /operation/start
func (rest *Rest) handleNewPrompt(w http.ResponseWriter, r *http.Request) {
	rest.logger.Debug("Add prompt request")
	// Создаем ответ
	var promptResp NewPromptResponse
	var errorAttrs ErrorAttributes

	w.Header().Set("Content-Type", "application/json")

	// Читаем тело запроса
	var promptReq NewPromptRequest
	err := json.NewDecoder(r.Body).Decode(&promptReq)
	if err != nil {
		errorAttrs.Code = "BadRequest"
		errorAttrs.Message = "Error parsing JSON request"
		promptResp.Error = errorAttrs
		sendJSONResponse(w, http.StatusBadRequest, promptResp)
		rest.logger.Error(errorAttrs.Message)
		rest.incrRequestMetric(METRIC_NEW_PROMPT, true)
		return
	}

	promptValue := promptmanager.Prompt{Prompt: promptReq.Prompt, Placeholders: nil}
	if promptReq.Negative != nil {
		promptValue.Negative = promptReq.Negative
	}

	err = rest.promptManager.AddNewPrompt(promptValue)
	if err != nil {
		errorAttrs.Code = "PromptError"
		errorAttrs.Message = "Can not add new prompt"
		errorAttrs.DevMessage = err.Error()
		promptResp.Error = errorAttrs
		sendJSONResponse(w, http.StatusBadRequest, promptResp)
		rest.logger.Error(errorAttrs.Message, slog.String("error", errorAttrs.DevMessage))
		rest.incrRequestMetric(METRIC_NEW_PROMPT, true)
		return
	}

	rest.incrRequestMetric(METRIC_NEW_PROMPT, false)
	promptResp.Status = opermanager.StatusDone
	sendJSONResponse(w, http.StatusCreated, promptResp)
}

func (rest *Rest) handleGetOperationStatus(w http.ResponseWriter, r *http.Request) {
	rest.logger.Debug("Handling GET operation status")
	var errorAttrs ErrorAttributes
	var statusResponse OperationStatusResponse

	vars := mux.Vars(r)
	operationId, ok := vars["operationId"]
	if !ok {
		errorAttrs.Code = "BadRequest"
		errorAttrs.Message = "operationId is missing in parameters"
		statusResponse.Error = errorAttrs
		sendJSONResponse(w, http.StatusBadRequest, statusResponse)
		rest.logger.Error(errorAttrs.Message)
		rest.incrRequestMetric(METRIC_OPERATION_STATUS, true)
		return
	}

	rest.logger.Debug("operationId " + operationId)

	statusResponse.ID = operationId

	status, err := rest.operMng.GetOperationStatus(operationId)
	if err != nil {
		errorAttrs.Code = "InternalError"
		errorAttrs.Message = "Can not get operation status"
		errorAttrs.DevMessage = err.Error()
		statusResponse.Error = errorAttrs
		sendJSONResponse(w, http.StatusUnprocessableEntity, statusResponse)
		rest.logger.Error(errorAttrs.Message, slog.String("error", errorAttrs.DevMessage))
		rest.incrRequestMetric(METRIC_OPERATION_STATUS, true)
		return
	}

	rest.logger.Debug("Status", slog.String("status", string(status.Status)), slog.String("error", status.Error), slog.String("operationId", operationId))

	statusResponse.Status = status.Status
	if len(status.Error) > 0 {
		errorAttrs.Code = "operationError"
		errorAttrs.Message = "operation have error status"
		errorAttrs.DevMessage = status.Error
		statusResponse.Error = errorAttrs
	}
	rest.incrRequestMetric(METRIC_OPERATION_STATUS, false)
	sendJSONResponse(w, http.StatusOK, statusResponse)
}

// Универсальная функция для отправки JSON-ответов
func sendJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			// Если не удалось закодировать данные в JSON, отправляем ошибку 500
			http.Error(w, "Error encoding JSON", http.StatusInternalServerError)
			return
		}
		w.Write(jsonData)
	}
}

func (rest *Rest) Start() error {
	certFile := "/certs/cert.pem"
	keyFile := "/certs/key.pem"

	addr := ":" + rest.port

	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		return fmt.Errorf("certificate not found: %s", certFile)
	}
	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		return fmt.Errorf("key not found: %s", keyFile)
	}

	return http.ListenAndServeTLS(addr, certFile, keyFile, rest.router)
}

func (rest *Rest) incrRequestMetric(metricType string, isError bool) {
	if isError {
		rest.metrics.IncrementErrorRequest(METRIC_ALL_WEB)
		rest.metrics.IncrementErrorRequest(metricType)
	} else {
		rest.metrics.IncrementSuccessRequest(METRIC_ALL_WEB)
		rest.metrics.IncrementSuccessRequest(metricType)
	}

}
