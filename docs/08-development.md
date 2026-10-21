# 8. Руководство разработчика

## 8.1. Требования к окружению

| Инструмент | Версия | Для чего |
|---|---|---|
| Docker + Compose v2 | Compose ≥ 2.17, BuildKit | запуск всего backend |
| Go | 1.24 | разработка и тесты сервисов |
| `protoc` + `protoc-gen-go` v1.36.5 + `protoc-gen-go-grpc` v1.5.1 | — | генерация gRPC-кода локально |
| Flutter | 3.47.5 stable | мобильное приложение |
| JDK | 17 (Temurin) | сборка Android |

Установка плагинов protoc:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.5
```

```bash
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
```

## 8.2. Локальный запуск backend

```bash
cp backend/.env.example backend/.env
```

Заполнить в `backend/.env` `JWT_SECRET`, `POSTGRES_PASSWORD`, `S3_SECRET_KEY`,
`RABBITMQ_PASSWORD` (и при желании `TELEGRAM_BOT_TOKEN`), затем:

```bash
cd backend && docker compose up --build
```

- При первом старте PostgreSQL применит все миграции.
- `price_updater` начнёт загрузку дневной истории за 365 дней в фоне,
  это несколько тысяч запросов к MOEX.
- API доступен на `http://localhost:8080`, RabbitMQ UI —
  `http://localhost:15672`, S3 — `localhost:9000`.

## 8.3. Соглашения по коду

### Go

- Форматирование — `gofmt`, статический анализ — `go vet` (проверяются в CI).
- Логирование — только `log/slog` в JSON.
- Ошибки внутренних сервисов — стандартные gRPC-коды (см.
  [таблицу соответствия](02-architecture.md#обработка-ошибок)).
- **Нет общих Go-пакетов между сервисами.** Каждый сервис — отдельный модуль;

### Dart

- Линтер — `flutter_lints`, проверка — `flutter analyze`.

### Git

- Ветка по умолчанию — `main`, изменения вливаются через PR.
- Сообщения коммитов: `Added: …`, `Updated: …`, `Fixed: …`.
- Релиз — тег `vX.Y.Z` на `main`.

## 8.4. Как добавить брокера

1. В `parser`: реализовать интерфейс `parsing.Parser`
   (`Parse(task.ReportUploaded, []byte) (parsing.Report, error)`) в
   `internal/parsing/<broker>`. У каждой строки должен быть стабильный
   `ExternalID`, одинаковый в любом отчёте, где эта строка встречается.
2. Зарегистрировать его в `cmd/main.go`: `parsers.Register("<broker>", …)`.
3. В `portfolio`: миграция со строкой в `brokers` (`id` = ключ парсера,
   `file_formats`, `icon_url`, `color`, `sort_order`).
4. Положить иконку в `gateway/internal/static/brokers/<broker>.png`
   (встраивается в бинарник).
5. Добавить тесты на реальных **обезличенных** отчётах в `testdata/`.
