# HTTP Load Balancer

Данный проект представляет собой HTTP-балансировщик нагрузки, реализованный на языке Go. Балансировщик распределяет входящие запросы по пулу бэкенд-серверов, обеспечивая высокую доступность и производительность.

## Основные возможности

- Распределение запросов по алгоритмам:
    - Round Robin (циклическое распределение)
    - Least Connections (наименьшее количество активных соединений)
    - Random (случайный выбор)
- Проверка доступности бэкендов (Health Checks)
- Ограничение скорости запросов (Rate Limiting) с использованием алгоритма Token Bucket
- API для управления клиентами и лимитами
- Graceful Shutdown для корректного завершения работы
- Подробное логирование всех действий

## Требования

- Go 1.24 или выше
- Redis (опционально, для хранения данных Rate Limiting)
- Docker и Docker Compose (для запуска в контейнерах)

## Структура проекта

```
loadbalancer/
├── cmd/
│   └── server/
│       └── main.go                # Точка входа в приложение
├── config/
│   ├── config.go                  # Структуры и функции для работы с конфигурацией
│   └── default.yaml               # Пример конфигурационного файла
├── internal/
│   ├── balancer/
│   │   ├── balancer.go            # Интерфейс балансировщика
│   │   └── roundrobin.go          # Алгоритмы балансировки
│   ├── proxy/
│   │   └── proxy.go               # Reverse proxy для перенаправления запросов
│   ├── ratelimit/
│   │   ├── tokenbucket.go         # Реализация алгоритма Token Bucket
│   │   └── client.go              # Управление клиентами
│   ├── storage/
│   │   ├── storage.go             # Интерфейс хранилища
│   │   ├── memory.go              # Реализация хранилища в памяти
│   │   └── redis.go               # Реализация хранилища на базе Redis
│   └── health/
│       └── checker.go             # Проверка доступности бэкендов
├── pkg/
│   ├── logger/
│   │   └── logger.go              # Настройка логирования
│   └── redis/
│       └── client.go              # Обертка для работы с Redis
├── Dockerfile                     # Для создания образа Docker
├── docker-compose.yml             # Для развертывания сервиса и БД
├── go.mod                         # Зависимости Go
└── README.md                      # Документация по проекту
```

## Конфигурация

Конфигурация балансировщика может выполняться через:

1. Конфигурационный файл YAML/JSON
2. Переменные окружения
3. Параметры командной строки

Пример конфигурационного файла:

```yaml
server:
  port: 8080
  timeout: 10s

logging:
  level: info       # debug, info, warn, error
  format: json      # json или console
  output: stdout    # stdout или file
  file_path: ./logs/balancer.log

backends: # для запуска в докер
  - url: http://backend1
  - url: http://backend2
  - url: http://backend3

balancer:
  algorithm: round_robin  # round_robin, least_connections, random

health_check:
  enabled: true
  interval: 5s
  path: /health

rate_limit:
  enabled: true
  redis:
    addr: localhost:6379
    password: ""
    db: 0
  default:
    capacity: 50       # Максимальная емкость бакета
    refill_rate: 10    # Токенов в секунду
```

Параметры можно переопределить через переменные окружения с префиксом `LB_`:

```
LB_SERVER_PORT=8080
LB_BALANCER_ALGORITHM=least_connections
LB_RATE_LIMIT_ENABLED=true
```

## Сборка и запуск

### Стандартная сборка

```bash
# Клонирование репозитория
git clone https://github.com/mihailpestrikov/go-cloud-camp-2025-test-assignment
cd go-cloud-camp-2025-test-assignment
```

### Запуск с помощью Docker

```bash
docker-compose up -d
```

## API

### Управление клиентами для Rate Limiting

#### Добавление/обновление клиента

```
POST /clients
Content-Type: application/json

{
  "client_id": "user1",
  "capacity": 100,
  "refill_rate": 10
}
```

#### Получение информации о клиенте

```
GET /clients?client_id=user1
```

#### Удаление клиента

```
DELETE /clients?client_id=user1
```

#### Статус клиента

```
GET /client-status?client_id=user1
```

Пример ответа:
```json
{
  "client_id": "user1",
  "capacity": 100,
  "refill_rate": 10,
  "tokens_remaining": 87,
  "tokens_percentage": 87
}
```

### Статус балансировщика

```
GET /lb-status
```

Пример ответа:
```json
{
  "status": "ok",
  "balancer": "round_robin",
  "backends": 3
}
```

### Статистика

```
GET /stats
```

Пример ответа:
```json
{
  "http://backend1": {
    "url": "http://backend1",
    "is_alive": true,
    "active_connections": 2,
    "total_requests": 175,
    "failed_requests": 3,
  },
  "http://backend2": {
    "url": "http://backend2",
    "is_alive": true,
    "active_connections": 1,
    "total_requests": 169,
    "failed_requests": 0,
  }
}
```

## Нагрузочное тестирование

Пример тестирования с помощью Apache Bench:

```bash
ab -n 5000 -c 1000 http://localhost:8080/
```

Пример тестирования с помощью hey:

```bash
hey -n 1000 -c 100 http://localhost:8080/
```

![Load Balancer Logo](/assets/images/lb.png)