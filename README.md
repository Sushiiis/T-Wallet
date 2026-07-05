# T-Wallet

Бэкенд электронного кошелька с переводами денег между пользователями.
Основной язык — Go. Внешний API: gRPC + REST-обёртка через grpc-gateway.

## Стек

Go, PostgreSQL (pgx v5), gRPC, grpc-gateway, JWT (HS256), bcrypt.
Позднее добавятся: Kafka (transactional outbox), Redis, Prometheus, OpenTelemetry.

## Архитектура

​```mermaid
graph LR
    Client -->|REST/JSON| Gateway[grpc-gateway]
    Client -->|gRPC| GRPC[gRPC Server]
    Gateway --> GRPC
    GRPC --> Usecase[Usecase layer]
    Usecase --> Repo[(PostgreSQL)]
    Usecase --> Redis[(Redis rate limit)]
    Repo --> Outbox[outbox table]
    Outbox -->|relay| Kafka[(Kafka)]
    Kafka --> Notifier[notifier consumer]
    Notifier --> Repo
    GRPC -.трейсы.-> Jaeger
    GRPC -.метрики.-> Prometheus
    Prometheus --> Grafana
​```

## Запуск локально

Требования: Docker, Go 1.24+, `protoc` в PATH.

```bash
cp .env.example .env         # заполните секреты
make up                      # поднять Postgres в Docker
make migrate-up              # применить миграции
make run                     # запустить сервис
```

По умолчанию:
- gRPC — `localhost:50051`
- REST — `http://localhost:8080`
- health-check — `GET /healthz`, `GET /readyz`

## Регенерация кода из .proto

```bash
make proto
```
Создаёт `.pb.go`, `_grpc.pb.go`, `.pb.gw.go` (REST-обёртка) и `.swagger.json`
(OpenAPI) в `api/proto/wallet/v1/`.

## Примеры запросов (REST)

Все запросы с телом требуют `Content-Type: application/json`.
Все денежные операции требуют заголовок `Idempotency-Key` (UUID).
Все методы кроме `Register` и `Login` требуют `Authorization: Bearer <JWT>`.

### Регистрация
```bash
curl -X POST http://localhost:8080/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"secret123"}'
```

### Логин (возвращает access_token)
```bash
curl -X POST http://localhost:8080/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"alice@example.com","password":"secret123"}'
```

### Создать счёт
```bash
curl -X POST http://localhost:8080/v1/accounts \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"currency":"RUB"}'
```

### Пополнение
```bash
curl -X POST http://localhost:8080/v1/accounts/$ACC/deposit \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"amount": 10000}'
```

### Баланс
```bash
curl http://localhost:8080/v1/accounts/$ACC/balance \
  -H "Authorization: Bearer $TOKEN"
```

### Списание
```bash
curl -X POST http://localhost:8080/v1/accounts/$ACC/withdraw \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"amount": 3000}'
```

### Перевод
```bash
curl -X POST http://localhost:8080/v1/transfers \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $(uuidgen)" \
  -H "Content-Type: application/json" \
  -d '{"from_account_id":"'$ACC'","to_account_id":"'$OTHER'","amount":500}'
```

## gRPC (для сравнения)

Reflection включён, `.proto` для клиента не нужен:
```bash
grpcurl -plaintext -H "authorization: Bearer $TOKEN" -H "idempotency-key: $(uuidgen)" \
  -d "{\"account_id\":\"$ACC\",\"amount\":10000}" \
  localhost:50051 wallet.v1.WalletService/Deposit
```

## Маппинг ошибок

gRPC-коды маппятся в HTTP-статусы grpc-gateway по стандартной таблице:

| Ситуация | gRPC | HTTP |
|---|---|---|
| Валидация (`amount ≤ 0`, `from == to`, нет `Idempotency-Key`) | `InvalidArgument` | 400 |
| Нет токена или он невалиден | `Unauthenticated` | 401 |
| Чужой счёт | `PermissionDenied` | 403 |
| Счёт/пользователь не найден | `NotFound` | 404 |
| Email уже занят / повтор `Idempotency-Key` с другим телом | `AlreadyExists` | 409 |
| Недостаточно средств | `FailedPrecondition` | 400 |

## OpenAPI-документация

Сгенерированный `api/proto/wallet/v1/wallet.swagger.json` можно вставить в
[Swagger Editor](https://editor.swagger.io) для просмотра всех эндпоинтов.