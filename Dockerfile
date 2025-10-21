FROM golang:1.25.3-alpine

WORKDIR /app

# Копируем go.mod и go.sum и загружаем зависимости
COPY src/ .

# Собираем приложение
RUN go build -o imgServer .

# Указываем, что контейнер должен запускать ваше приложение
CMD ["./imgServer"]