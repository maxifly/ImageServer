package rest

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"html/template"
	"imgserver/internal/pkg/helpers"
	"imgserver/internal/pkg/localimageprovider"
	"imgserver/internal/pkg/metrics"
	"imgserver/internal/pkg/opermanager"
	"imgserver/internal/pkg/promptmanager"
	"imgserver/internal/pkg/ydart"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

	fileServer := http.FileServer(http.Dir("./internal/pkg/rest/ui/static/"))
	router.PathPrefix("/static/").Handler(http.StripPrefix("/static", fileServer))

	router.HandleFunc("/", restObj.handleIndex).Methods("GET")
	router.HandleFunc("/index", restObj.handleIndex).Methods("GET")
	router.HandleFunc("/api/status", restObj.handleStatusAPI).Methods("GET")
	router.HandleFunc("/api/prompts", restObj.handleGetPromptsPage).Methods("GET")
	router.HandleFunc("/api/prompts", restObj.handleCreatePrompt).Methods("POST")
	router.HandleFunc("/api/prompts/{promptId}", restObj.handlePromptByIdApi).Methods("GET", "PUT")
	router.HandleFunc("/api/prompts/{promptId}", restObj.handleDeletePromptByIdApi).Methods("DELETE")
	router.HandleFunc("/api/global-placeholders", restObj.handleCreateGlobalPlaceholder).Methods("POST")
	router.HandleFunc("/api/global-placeholders/{name}", restObj.handleGlobalPlaceholderByIdApi).Methods("GET", "PUT")
	router.HandleFunc("/api/global-placeholders/{name}", restObj.handleDeleteGlobalPlaceholderByIdApi).Methods("DELETE")
	router.HandleFunc("/api/generate", restObj.handleGenerateApi).Methods("POST")

	router.HandleFunc("/operation/start", restObj.handleStartOperation).Methods("POST")
	router.HandleFunc("/operation/status/{operationId}", restObj.handleGetOperationStatus).Methods("GET")
	router.HandleFunc("/operation/result/{operationId}", restObj.handleGetImage).Methods("GET")
	router.HandleFunc("/prompt/add", restObj.handleNewPrompt).Methods("POST")

	logger.Error("(It is not error!!!) Run WEB-Server on https://127.0.0.1", "port", port)

	return &restObj, nil

}

func (rest *Rest) handleIndex(w http.ResponseWriter, r *http.Request) {

	rest.logger.Info("indexHandler")
	files := []string{
		"./internal/pkg/rest/ui/html/index.html",
		"./internal/pkg/rest/ui/html/base.html",
	}

	ts, err := template.ParseFiles(files...)
	if err != nil {
		rest.logger.Error("Error parse files", "error", err)
		http.Error(w, "Internal Server Error", 500)
		return
	}

	data := rest.getPageData()

	err = ts.Execute(w, data)
	if err != nil {
		rest.logger.Error("Error execute template", "error", err)
		http.Error(w, "Internal Server Error", 500)
	}
}

func (rest *Rest) handleStatusAPI(w http.ResponseWriter, r *http.Request) {
	rest.logger.Debug("Status API")
	data := rest.getPageData()

	sendJSONResponse(w, http.StatusOK, data)
	//w.Header().Set("Content-Type", "application/json")
	//json.NewEncoder(w).Encode(data)
}

func (rest *Rest) getPrompts() *promptmanager.PromptsData {
	return rest.promptManager.GetPromptsData()
}

func (rest *Rest) getPageData() StatusResponse {
	alertMessages := make([]AlertMessage, 0)

	var groups []MetricGroup

	groups = append(groups,
		MetricGroup{
			ID:          1,
			Name:        "Total",
			TotalCount:  rest.metrics.GetRequestTypeMetricsSafe(METRIC_ALL_WEB).Total.Count(),
			ErrorCount:  rest.metrics.GetRequestTypeMetricsSafe(METRIC_ALL_WEB).Errors.Count(),
			SuccessRate: helpers.RoundToTwoDecimals(rest.metrics.GetRequestTypeMetricsSafe(METRIC_ALL_WEB).SuccessRate.Rate15() * 3600.),
			ErrorRate:   helpers.RoundToTwoDecimals(rest.metrics.GetRequestTypeMetricsSafe(METRIC_ALL_WEB).ErrorRate.Rate15() * 3600.),
		})

	groups = append(groups,
		MetricGroup{
			ID:          2,
			Name:        "Image send",
			TotalCount:  rest.metrics.GetRequestTypeMetricsSafe(METRIC_IMAGE_GET).Total.Count(),
			ErrorCount:  rest.metrics.GetRequestTypeMetricsSafe(METRIC_IMAGE_GET).Errors.Count(),
			SuccessRate: helpers.RoundToTwoDecimals(rest.metrics.GetRequestTypeMetricsSafe(METRIC_IMAGE_GET).SuccessRate.Rate15() * 3600.),
			ErrorRate:   helpers.RoundToTwoDecimals(rest.metrics.GetRequestTypeMetricsSafe(METRIC_IMAGE_GET).ErrorRate.Rate15() * 3600.),
		})

	var providerGroups []MetricGroup
	ydArtMetric := rest.metrics.GetRequestTypeMetricsSafe(opermanager.METRIC_TEMPLATE_OPERATION_START + ydart.ProviderCode)
	providerGroups = append(providerGroups,
		MetricGroup{
			ID:          1,
			Name:        "YandexArt",
			TotalCount:  ydArtMetric.Total.Count(),
			ErrorCount:  ydArtMetric.Errors.Count(),
			SuccessRate: helpers.RoundToTwoDecimals(ydArtMetric.SuccessRate.Rate15() * 3600.),
			ErrorRate:   helpers.RoundToTwoDecimals(ydArtMetric.ErrorRate.Rate15() * 3600.),
		})

	limMetric := rest.metrics.GetRequestTypeMetricsSafe(opermanager.METRIC_TEMPLATE_OPERATION_START + localimageprovider.ProviderCode)
	providerGroups = append(providerGroups,
		MetricGroup{
			ID:          2,
			Name:        "LocalImage",
			TotalCount:  limMetric.Total.Count(),
			ErrorCount:  limMetric.Errors.Count(),
			SuccessRate: helpers.RoundToTwoDecimals(limMetric.SuccessRate.Rate15() * 3600.),
			ErrorRate:   helpers.RoundToTwoDecimals(limMetric.ErrorRate.Rate15() * 3600.),
		})

	fileMetrics := rest.metrics.GetAllFileMetrics()
	var fileAmounts []FileAmount

	for k, v := range fileMetrics {
		fileAmounts = append(fileAmounts, FileAmount{k, v.Value()})
	}

	sort.SliceStable(fileAmounts, func(i, j int) bool {
		return fileAmounts[i].DirType < fileAmounts[j].DirType
	})

	return StatusResponse{
		AlertMessages:   alertMessages,
		Groups:          groups,
		ProviderGroups:  providerGroups,
		FileAmounts:     fileAmounts,
		YandexYesterday: rest.metrics.GetDailyMetricSafe(time.Now().Add(-time.Duration(24)*time.Hour), opermanager.METRIC_TEMPLATE_OPERATION_START+ydart.ProviderCode).Counter.Count(),
		YandexToday:     rest.metrics.GetDailyMetricSafe(time.Now(), opermanager.METRIC_TEMPLATE_OPERATION_START+ydart.ProviderCode).Counter.Count(),
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

// Функция для обработки POST-запросов
func (rest *Rest) handleGenerateApi(w http.ResponseWriter, r *http.Request) {
	rest.logger.Debug("Generate by prompt")
	var generateReq GenerateByPromptRequest
	err := json.NewDecoder(r.Body).Decode(&generateReq)
	if err != nil {
		rest.logger.Error("Cannot parse body", "error", err)
		http.Error(w, "Cannot parse body", http.StatusUnprocessableEntity)
		return
	}

	//TODO create

	w.WriteHeader(http.StatusOK)
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

	_, err = rest.promptManager.AddNewPrompt(promptValue)
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

func (rest *Rest) handleGetPromptsPage(w http.ResponseWriter, r *http.Request) {
	rest.logger.Info("getPromptsPage")
	files := []string{
		"./internal/pkg/rest/ui/html/prompts.html",
		"./internal/pkg/rest/ui/html/base.html",
	}
	mainName := filepath.Base(files[0])

	ts, err := template.New(mainName).Funcs(template.FuncMap{
		"jsonify": jsonify,
	}).ParseFiles(files...)

	if err != nil {
		rest.logger.Error("Error parse files", "error", err)
		http.Error(w, "Internal Server Error", 500)
		return
	}

	prompts := rest.getPrompts()
	cards := make([]PromptCardResponse, 0, len(prompts.Prompts))

	rest.logger.Error("***", "len", len(prompts.Prompts))

	for _, p := range prompts.Prompts {
		card := PromptCardResponse{
			ID:              p.Idx,
			Text:            p.Prompt,
			PlaceholderKeys: JoinMapKeys(p.Placeholders),
		}
		cards = append(cards, card)
		sort.Slice(cards, func(i, j int) bool {
			return cards[i].ID < cards[j].ID
		})
	}

	globalPlaceholders := make([]PlaceholderDetail, 0, len(prompts.GlobalPlaceholders))
	for gpName, gpv := range prompts.GlobalPlaceholders {
		gpd := PlaceholderDetail{
			Name:   gpName,
			Values: gpv,
		}
		globalPlaceholders = append(globalPlaceholders, gpd)
	}

	data := PromptsPageResponse{
		Prompts:            cards,
		GlobalPlaceholders: globalPlaceholders,
	}

	data.Providers = append(data.Providers, "YdArt")

	rest.logger.Debug("*** cards", "len", len(prompts.Prompts))
	rest.logger.Debug("*** globalPlaceholders", "len", len(prompts.GlobalPlaceholders))

	err = ts.Execute(w, data)
	if err != nil {
		rest.logger.Error("Error execute template", "error", err)
		http.Error(w, "Internal Server Error", 500)
	}
}

func JoinMapKeys(m map[string][]string) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys) // опционально, но полезно
	return keys
}
func (rest *Rest) handleCreateGlobalPlaceholder(w http.ResponseWriter, r *http.Request) {
	rest.logger.Info("Create global placeholder from api")

	var placeholderReq PlaceholderDetail
	err := json.NewDecoder(r.Body).Decode(&placeholderReq)
	if err != nil {
		rest.logger.Error("Cannot parse body", "error", err)
		http.Error(w, "Cannot parse body", http.StatusUnprocessableEntity)
		return
	}

	err = rest.promptManager.AddGlobalPlaceholder(placeholderReq.Name, placeholderReq.Values)
	if err != nil {
		rest.logger.Error("Cannot add global placeholder", "error", err)
		http.Error(w, "Cannot add global placeholder: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.WriteHeader(http.StatusOK)
	return
}
func (rest *Rest) handleCreatePrompt(w http.ResponseWriter, r *http.Request) {

	rest.logger.Info("Create prompt from api")

	var promptReq PromptDetailRequest
	err := json.NewDecoder(r.Body).Decode(&promptReq)
	if err != nil {
		rest.logger.Error("Cannot parse body", "error", err)
		http.Error(w, "Cannot parse body", http.StatusUnprocessableEntity)
		return
	}

	prompt := promptmanager.Prompt{
		Idx:      promptReq.ID,
		Prompt:   promptReq.Text,
		Negative: promptReq.Negative,
	}

	if promptReq.Placeholders != nil {
		placeholders := make(map[string][]string)
		for _, ph := range promptReq.Placeholders {
			values := make([]string, 0, len(ph.Values))
			for _, value := range ph.Values {
				values = append(values, value)
			}
			placeholders[ph.Name] = values
		}
		prompt.Placeholders = placeholders
	}

	newID, err := rest.promptManager.AddNewPrompt(prompt)
	if err != nil {
		rest.logger.Error("Cannot add prompt", "error", err)
		http.Error(w, "Cannot add prompt: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]int{"ID": newID}); err != nil {
		log.Printf("Ошибка кодирования JSON: %v", err)
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
	}
	w.WriteHeader(http.StatusOK)
	return

	//var input struct {
	//	Text        string            `json:"Text"`
	//	Placeholders []PlaceholderDetail `json:"Placeholders"`
	//}
	//if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
	//	http.Error(w, "Неверный JSON", http.StatusBadRequest)
	//	return
	//}
	//
	//// Генерация нового ID (например, UUID или ULID)
	//newID := generateID() // ← реализуй свою логику
	//
	//newPrompt := Prompt{
	//	ID:          newID,
	//	Text:        input.Text,
	//	Placeholders: input.Placeholders,
	//}
	//
	//// Сохранение в хранилище
	//promptStore[newID] = &newPrompt
	//
	//// Ответ: можно вернуть ID или весь объект
	//w.Header().Set("Content-Type", "application/json")
	//json.NewEncoder(w).Encode(map[string]string{"ID": newID})
}

func (rest *Rest) handleDeletePromptByIdApi(w http.ResponseWriter, r *http.Request) {
	rest.logger.Info("Delete prompt by id")
	vars := mux.Vars(r)
	promptIdS, ok := vars["promptId"]
	if !ok {
		rest.logger.Error("Prompt id is empty")
		http.Error(w, "Prompt ID is empty", http.StatusBadRequest)
		return
	}

	promptId, err := strconv.Atoi(promptIdS)
	if err != nil {
		http.Error(w, "Prompt ID must be integer", http.StatusBadRequest)
		return
	}

	err = rest.promptManager.DeletePrompt(promptId)
	if err != nil {
		rest.logger.Error("Cannot delete prompt", "error", err)
		http.Error(w, "Cannot delete prompt: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.WriteHeader(http.StatusOK)
	return
}

func (rest *Rest) handleDeleteGlobalPlaceholderByIdApi(w http.ResponseWriter, r *http.Request) {
	rest.logger.Info("Delete global placeholder by id")
	vars := mux.Vars(r)

	placeholderName, ok := vars["name"]
	if !ok {
		rest.logger.Error("Placeholder name is empty")
		http.Error(w, "Placeholder name is empty", http.StatusBadRequest)
		return
	}

	err := rest.promptManager.DeleteGlobalPlaceholder(placeholderName)

	if err != nil {
		rest.logger.Error("Cannot delete global placeholder", "error", err)
		http.Error(w, "Cannot delete global placeholder: "+err.Error(), http.StatusUnprocessableEntity)
		return
	}

	w.WriteHeader(http.StatusOK)
	return
}

func (rest *Rest) handleGlobalPlaceholderByIdApi(w http.ResponseWriter, r *http.Request) {
	rest.logger.Info("Processing global placeholder by id")
	vars := mux.Vars(r)

	placeholderName, ok := vars["name"]
	if !ok {
		rest.logger.Error("Placeholder name is empty")
		http.Error(w, "Placeholder name is empty", http.StatusBadRequest)
		return
	}
	if r.Method == http.MethodGet {

		values, exists := rest.promptManager.GetPlaceholderValuesById(placeholderName)
		if !exists {
			http.Error(w, "Placeholder not found", http.StatusNotFound)
			return
		}

		result := PlaceholderDetail{
			Name:   placeholderName,
			Values: values,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			log.Printf("Ошибка кодирования JSON: %v", err)
			http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		}
		return
	}

	if r.Method == http.MethodPut {
		var placeholderReq PlaceholderDetail
		err := json.NewDecoder(r.Body).Decode(&placeholderReq)
		if err != nil {
			rest.logger.Error("Cannot parse body", "error", err)
			http.Error(w, "Cannot parse body", http.StatusUnprocessableEntity)
			return
		}

		err = rest.promptManager.ChangeGlobalPlaceholder(placeholderReq.Name, placeholderReq.Values)
		if err != nil {
			rest.logger.Error("Cannot change global placeholder", "error", err)
			http.Error(w, "Cannot change global placeholder: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)

}
func (rest *Rest) handlePromptByIdApi(w http.ResponseWriter, r *http.Request) {
	rest.logger.Info("Processing prompt by id")
	vars := mux.Vars(r)

	promptIdS, ok := vars["promptId"]
	if !ok {
		rest.logger.Error("Prompt id is empty")
		http.Error(w, "Prompt ID is empty", http.StatusBadRequest)
		return
	}

	promptId, err := strconv.Atoi(promptIdS)
	if err != nil {
		http.Error(w, "Prompt ID must be integer", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodGet {

		prompt, exists := rest.promptManager.GetPromptById(promptId)
		if !exists {
			http.Error(w, "Не найдено", http.StatusNotFound)
			return
		}

		placeholders := make([]PlaceholderDetail, 0)

		if prompt.Placeholders != nil {
			for key, values := range prompt.Placeholders {
				sort.Strings(values)
				phd := PlaceholderDetail{
					Name:   key,
					Values: values,
				}
				placeholders = append(placeholders, phd)
			}
			sort.Slice(placeholders, func(i, j int) bool {
				return placeholders[i].Name < placeholders[j].Name
			})
		}

		result := PromptDetail{
			ID:           prompt.Idx,
			Text:         prompt.Prompt,
			Negative:     prompt.Negative,
			Placeholders: placeholders,
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(result); err != nil {
			log.Printf("Ошибка кодирования JSON: %v", err)
			http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		}
		return
	}

	if r.Method == http.MethodPut {
		var promptReq PromptDetailRequest
		err := json.NewDecoder(r.Body).Decode(&promptReq)
		if err != nil {
			rest.logger.Error("Cannot parse body", "error", err)
			http.Error(w, "Cannot parse body", http.StatusUnprocessableEntity)
			return
		}

		prompt := promptmanager.Prompt{
			Idx:      promptReq.ID,
			Prompt:   promptReq.Text,
			Negative: promptReq.Negative,
		}

		if promptReq.Placeholders != nil {
			placeholders := make(map[string][]string)
			for _, ph := range promptReq.Placeholders {
				values := make([]string, 0, len(ph.Values))
				for _, value := range ph.Values {
					values = append(values, value)
				}
				placeholders[ph.Name] = values
			}
			prompt.Placeholders = placeholders
		}

		err = rest.promptManager.ChangePrompt(prompt)
		if err != nil {
			rest.logger.Error("Cannot change prompt", "error", err)
			http.Error(w, "Cannot change prompt: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
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

func jsonify(v interface{}) template.JS {
	data, err := json.Marshal(v)
	if err != nil {
		return template.JS("null")
	}
	return template.JS(data)
}
