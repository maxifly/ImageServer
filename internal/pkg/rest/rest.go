package rest

import (
	"github.com/gorilla/mux"
	"html/template"
	"log/slog"
	"net/http"
)

// StatusResponse структура для отображения статуса
type StatusResponse struct {
	TotalRequests int `json:"total_requests"`
	ImagesSent    int `json:"images_sent"`
}

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
	logger *slog.Logger
	router *mux.Router
	port   string
}

func NewRest(port string,
	logger *slog.Logger) (*Rest, error) {

	router := mux.NewRouter()

	restObj := Rest{port: port,
		router: router,
		logger: logger,
	}

	router.HandleFunc("/", restObj.handleIndex).Methods("GET")
	router.HandleFunc("/operation/{fileName}", restObj.handleGetImage).Methods("GET")

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
	fileName, ok := vars["fileName"]
	if !ok {
		rest.logger.Error("fileName is missing in parameters")
	}

	rest.logger.Debug("File name " + fileName)

}

func (rest *Rest) Start() error {
	return http.ListenAndServe(":"+rest.port, rest.router)
}
