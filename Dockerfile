FROM golang:1.25.3-alpine

RUN apk add --no-cache tzdata

ARG TZ=UTC
ENV TZ=${TZ}

RUN ln -sf /usr/share/zoneinfo/${TZ} /etc/localtime 2>/dev/null || true


WORKDIR /app

# Копируем go.mod и go.sum и загружаем зависимости
COPY src/ .

# Собираем приложение
RUN go build -o imgServer .

# Указываем, что контейнер должен запускать ваше приложение
CMD ["./imgServer"]