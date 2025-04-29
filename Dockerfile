# Этап сборки
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Установка необходимых пакетов
RUN apk add --no-cache git ca-certificates tzdata

# Копируем файлы модулей и скачиваем зависимости
COPY go.mod go.sum ./
RUN go mod download

# Копируем исходный код
COPY . .

# Сборка приложения
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o load-balancer ./cmd/server

# Финальный этап
FROM alpine:latest

WORKDIR /app

# Копируем исполняемый файл и конфигурацию из этапа сборки
COPY --from=builder /app/load-balancer .
COPY --from=builder /app/config/default.yaml /app/config/default.yaml
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Создаем директорию для логов
RUN mkdir -p /app/logs

# Создаем непривилегированного пользователя для запуска приложения
RUN adduser -D -g '' appuser
RUN chown -R appuser:appuser /app
USER appuser

# Порт для HTTP-сервера
EXPOSE 8080

# Запускаем приложение
ENTRYPOINT ["./load-balancer"]
CMD ["--config", "/app/config/default.yaml"]