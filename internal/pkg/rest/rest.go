package rest

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"html/template"
	"imgserver/internal/pkg/opermanager"
	"log/slog"
	"net/http"
)

// Шаблон для веб-страницы
var indexTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>Image Server Status</title>
</head>
<body>
    <h1>Image Server Status</h1>
    <p>Total Requests: {{.TotalRequests}}</p>
    <p>Images Sent: {{.ImagesSent}}</p>
</body>
</html>
`

type Rest struct {
	logger  *slog.Logger
	router  *mux.Router
	operMng *opermanager.OperMngr
	port    string
}

func NewRest(port string,
	logger *slog.Logger,
	operMng *opermanager.OperMngr,
) (*Rest, error) {

	router := mux.NewRouter()

	restObj := Rest{port: port,
		router:  router,
		logger:  logger,
		operMng: operMng,
	}

	router.HandleFunc("/", restObj.handleIndex).Methods("GET")
	router.HandleFunc("/operation/start", restObj.handleGetImage).Methods("POST")
	router.HandleFunc("/operation/status/{operationId}", restObj.handleGetOperationStatus).Methods("GET")
	router.HandleFunc("/operation/result/{operationId}", restObj.handleGetImage).Methods("GET")

	logger.Error("(It is not error!!!) Run WEB-Server on http://127.0.0.1:%s", port)

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

	//// Блокируем доступ к счетчикам
	//mu.Lock()
	//defer mu.Unlock()

	// Выполняем шаблон
	err = tmpl.Execute(w, StatusResponse{
		TotalRequests: 0,
		ImagesSent:    0,
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
		return
	}

	rest.logger.Debug("Operation id " + operationId)

	//TODO Както получить имя файла

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
		return
	}

	// TODO Проверяем тип операции
	//if startReq.Type != "auto" {
	//	http.Error(w, "Invalid operation type", http.StatusBadRequest)
	//	return
	//}

	//TODO Yадо как-то стартовать операцию разного типа
	operationId, err := rest.operMng.StartOperation()
	if err != nil {
		errorAttrs.Code = "StartError"
		errorAttrs.Message = "Can not start operation"
		errorAttrs.DevMessage = err.Error()
		startResp.Error = errorAttrs
		sendJSONResponse(w, http.StatusBadRequest, startResp)
		rest.logger.Error(errorAttrs.Message, slog.String("error", errorAttrs.DevMessage))
		return
	}

	startResp.ID = operationId
	sendJSONResponse(w, http.StatusOK, startResp)
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
		return
	}

	rest.logger.Debug("Status "+status.Status, slog.String("error", status.Error), slog.String("operationId", operationId))

	statusResponse.Status = status.Status
	if len(status.Error) > 0 {
		errorAttrs.Code = "operationError"
		errorAttrs.Message = "operation have error status"
		errorAttrs.DevMessage = status.Error
		statusResponse.Error = errorAttrs
	}
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
	return http.ListenAndServe(":"+rest.port, rest.router)
}
