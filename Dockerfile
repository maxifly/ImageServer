# ============================================
# Этап 1: Сборка
# ============================================
FROM golang:1.25.7-alpine AS builder

RUN apk add --no-cache tzdata git

ARG TZ=UTC
ENV TZ=${TZ}
RUN ln -sf /usr/share/zoneinfo/${TZ} /etc/localtime 2>/dev/null || true

WORKDIR /app

# Сначала зависимости (кэш Docker)
COPY src/go.mod src/go.sum ./
RUN go mod download

# Потом исходники и миграции
COPY src/ .
#COPY migrations/ ./migrations/

# Сборка бинарника
# CGO_ENABLED=0 — для modernc.org/sqlite (чистый Go)
# Если mattn/go-sqlite3 — замени на CGO_ENABLED=1 и добавь gcc musl-dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o imgServer .

# ============================================
# Этап 2: Runtime
# ============================================
FROM alpine:latest

RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    sqlite

WORKDIR /app

# Копируем только бинарник и миграции
COPY --from=builder /app/imgServer .
COPY --from=builder /app/migrations ./migrations/
COPY src/internal/pkg/rest/ui ./internal/pkg/rest/ui/

# Часовая зона
ARG TZ=UTC
ENV TZ=${TZ}
RUN ln -sf /usr/share/zoneinfo/${TZ} /etc/localtime 2>/dev/null || true

# Healthcheck — проверка целостности БД
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD sqlite3 /data/db/myapp.db "PRAGMA integrity_check;" | grep -q "ok" || exit 1

# Точка входа (exec form — сигналы доходят напрямую)
ENTRYPOINT ["./imgServer"]