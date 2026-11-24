# Внутренние контракты

Синхронное взаимодействие внутри docker-сети идёт по gRPC, асинхронное —
через сообщения RabbitMQ. Внутренние контракты не публичны: их клиенты —
только `gateway` и `parser`.

## Общие правила gRPC

| Правило | Значение |
|---|---|
| Транспорт | gRPC поверх HTTP/2 без TLS (`-plaintext`) внутри docker-сети |
| Пакеты | `invest.<service>.v1` |
| Источник `.proto` | только у сервиса-владельца: `backend/services/<owner>/proto/*.proto` |
| Идентификация | metadata `x-user-id: <uuid>` выставляет `gateway` (или `parser` из задачи) |
| Ошибки | стандартные коды `google.golang.org/grpc/codes`; соответствие HTTP — в [таблице соответствия gRPC и HTTP](../02-architecture.md#обработка-ошибок) |
| Health | `grpc.health.v1.Health/Check` у каждого gRPC-сервиса |
| Reflection | включён: `grpcurl -plaintext <host:port> list` |
| Совместимость | только обратно совместимые изменения: новые поля с новыми номерами, удалённые номера не переиспользуются |

## users

`invest.users.v1.UsersService` · порт 8082 · [`users.proto`](../../backend/services/users/proto/users.proto)

| RPC | `x-user-id` | Описание | Основные ошибки |
|---|---|---|---|
| `Register(RegisterRequest) → User` | нет | создать аккаунт | `INVALID_ARGUMENT`, `ALREADY_EXISTS` |
| `Login(LoginRequest) → LoginResponse` | нет | выдать пару токенов, новая семья | `UNAUTHENTICATED` |
| `RefreshToken(RefreshTokenRequest) → LoginResponse` | нет | атомарная ротация, обнаружение повторного использования | `UNAUTHENTICATED` |
| `Logout(LogoutRequest) → Empty` | нет | отозвать refresh-токен, идемпотентно | `INVALID_ARGUMENT` |
| `GetMe(Empty) → User` | да | текущий пользователь | `UNAUTHENTICATED`, `NOT_FOUND` |
| `GetUser(GetUserRequest) → User` | да | пользователь по id | `NOT_FOUND` |
| `UpdateMe(UpdateMeRequest) → User` | да | сменить email и/или логин | `INVALID_ARGUMENT`, `ALREADY_EXISTS`, `NOT_FOUND` |
| `ChangePassword(ChangePasswordRequest) → LoginResponse` | да | сменить пароль, отозвать все сессии | `PERMISSION_DENIED`, `INVALID_ARGUMENT` |
| `DeleteMe(DeleteMeRequest) → Empty` | да | удалить аккаунт | `PERMISSION_DENIED` |

## portfolio

`invest.portfolio.v1.PortfolioService` · порт 8083 · [`portfolio.proto`](../../backend/services/portfolio/proto/portfolio.proto) ·
все RPC требуют `x-user-id`.

| RPC | Клиент | Описание |
|---|---|---|
| `CreatePortfolio` | gateway | обычный или составной (`member_ids`) портфель |
| `ListPortfolios`, `GetPortfolio` | gateway | чтение; чужой портфель — `NOT_FOUND` |
| `UpdatePortfolio` | gateway | переименование (1–100 символов) |
| `CreateTrade` | gateway | ручной ввод сделки; неизвестная бумага — `FAILED_PRECONDITION` |
| `CreateTrades` | — (не используется parser с 2026-09-23) | пачка сделок в одной транзакции; повтор `external_id` — `ALREADY_EXISTS` |
| `ListTrades`, `ListCashOperations` | gateway | журналы (у составного — всех участников) |
| `GetHoldings`, `GetPnL` | gateway | расчёт позиций и P&L |
| `GetValueHistory` | gateway | стоимость по дням: `week`/`month`/`year`/`all` |
| `ListBrokers` | gateway | включённые брокеры по `sort_order` |
| `CreateReportImport` | gateway | создать запись импорта (`id` = `task_id`) с проверками портфеля, брокера и расширения файла |
| `GetReportImport`, `ListReportImports` | gateway | статус импорта |
| `UpdateReportImportStatus` | gateway, parser | `processing` / `failed` / `done`; завершённый импорт не меняется (`FAILED_PRECONDITION`) |
| `ImportReport` | parser | сохранить весь отчёт в одной транзакции, идемпотентно |

### `ImportReport` — детали

```protobuf
message ImportReportRequest {
  string portfolio_id = 1;
  repeated TradeInput trades = 2;
  repeated CashOperationInput cash_operations = 3;
  string report_import_id = 4;                 // импорт помечается done в той же транзакции
  string account_key = 5;                      // "sber:S24RH4S" — общий префикс external_id строк
  google.protobuf.Timestamp period_start = 6;  // начало периода отчёта
}
```

- Строка с уже существующим в портфеле `external_id` пропускается
  (`ON CONFLICT DO NOTHING`). Ответ содержит счётчики
  `*_created` / `*_skipped`.
- Сделка по бумаге, которой нет в `securities`, — `FAILED_PRECONDITION`,
  весь импорт откатывается. У денежных операций FK на бумагу нет.
- **Вводные остатки.** Строки с `external_id`, начинающимся с
  `<account_key>:opening:`, — бумаги и деньги на начало периода. Они
  импортируются, только если в портфеле нет более ранних строк этого
  счёта. Вводные строки этого счёта с датой позже `period_start` удаляются:
  новый отчёт приносит настоящую историю за это время.
- Пустой отчёт или неизвестный тип операции — `INVALID_ARGUMENT`.

## securities_reader

`invest.securitiesreader.v1.SecuritiesReaderService` · порт 8081 ·
[`securitiesreader.proto`](../../backend/services/securities_reader/proto/securitiesreader.proto) ·
`x-user-id` не требуется (доступ ограничивает gateway).

| RPC | Описание |
|---|---|
| `GetPrices(tickers)` | последние цены; пустой список — все бумаги |
| `GetCandles(secid, board, range)` | свечи; `CandleRange`: `DAY`, `WEEK`, `MONTH`, `YEAR`, `FIVE_YEARS`, `ALL`; `CandleInterval` в ответе: `TEN_MINUTES`, `HOUR`, `DAY`, `WEEK` |
| `SearchSecurities(query, limit)` | поиск с ранжированием, `limit` 0 → 20, максимум 50 |
| `GetSecurityInfo(secid, board)` | поля описания бумаги из БД (обновляет price_updater) |
| `GetDividends(secid)` | дивиденды из БД, признак `forecast` |

## notifier

`invest.notifier.v1.NotifierService` · порт 8085 ·
[`notifier.proto`](../../backend/services/notifier/proto/notifier.proto) ·
все RPC требуют `x-user-id`.

| RPC | Описание |
|---|---|
| `CreateAlert(secid, board, oneof{target_price, change_percent})` | создать алерт от текущей цены |
| `ListAlerts` | алерты пользователя |
| `UpdateAlert(id, oneof{…}, secid?, board?)` | новая цель и/или бумага, статус снова `ACTIVE` |
| `DeleteAlert(id)` | удалить |
| `CreateTelegramLink` | одноразовая ссылка `t.me/<bot>?start=<token>` |
| `GetTelegramLink` | состояние привязки и `bot_enabled` |
| `DeleteTelegramLink` | отвязать чат |

Перечисления: `Direction` = `ABOVE` (`price >= target`) / `BELOW`
(`price <= target`); `AlertStatus` = `ACTIVE` / `TRIGGERED`.

## feed

`invest.feed.v1.FeedService` · порт 8086 ·
[`feed.proto`](../../backend/services/feed/proto/feed.proto).
Gateway передаёт проверенный идентификатор пользователя через `x-user-id`.

| RPC | Назначение |
|---|---|
| `CreatePost` | Валидирует новость или прогноз и сохраняет публикацию. Для прогноза фиксирует текущую цену. |
| `ListPosts` | Возвращает новости и активные прогнозы с фильтрами и курсором по времени публикации и ID. |

## parser (health)

Собственного API нет. Порт 8084 — только `grpc.health.v1.Health/Check`,
проверяет RabbitMQ и S3.

## Сообщение RabbitMQ `report.uploaded`

| Параметр | Значение |
|---|---|
| Exchange | default (`""`), routing key = имя очереди |
| Очередь | `report.uploaded` |
| Издатель | `gateway` (`internal/task.ReportUploaded`) |
| Потребитель | `parser` (`internal/task.ReportUploaded` — намеренная копия, а не общий пакет) |
| Семантика | at-least-once; `prefetch=1`; повтор безопасен благодаря идемпотентному `ImportReport` |
| Dead-letter | не настроен |

```json
{
  "task_id": "b6e6c6b0-…",
  "user_id": "3fa2…",
  "portfolio_id": "9c11…",
  "broker": "sber",
  "bucket": "reports",
  "object_key": "<portfolio_id>/<task_id>_S24RH4S_01082026_31082026.HTML",
  "filename": "S24RH4S_01082026_31082026.HTML",
  "content_type": "text/html",
  "uploaded_at": "2026-09-16T12:00:00Z"
}
```

| Поле | Описание |
|---|---|
| `task_id` | UUID задачи, он же id записи `report_imports` |
| `user_id` | владелец портфеля; `parser` передаёт его как `x-user-id` |
| `portfolio_id` | куда импортировать; владение уже проверил `gateway` |
| `broker` | нормализованный ключ брокера (lowercase, пробелы → `-`) |
| `bucket`, `object_key` | где лежит файл в S3 |
| `filename`, `content_type` | исходное имя и тип файла |
| `uploaded_at` | время загрузки (UTC) |

Изменять формат сообщения нужно синхронно в обоих сервисах. Новые поля
допустимы (JSON-декодер игнорирует неизвестные).

## Внешние API, которые потребляет система

| API | Потребитель | Эндпоинты |
|---|---|---|
| MOEX ISS | price_updater | `GET /engines/stock/markets/{market}/boards/{board}/securities.json`, `GET /history/engines/stock/markets/{market}/boards/{board}/securities.json?date=…`, `GET /engines/stock/markets/{market}/boards/{board}/securities/{secid}/candles.json?interval=10&from=…&till=…&start=…` |
| MOEX ISS (метаданные) | price_updater | `GET /securities/{secid}.json` (описание), `GET /securities.json` |
| dohod.ru | price_updater | `https://www.dohod.ru/ik/analytics/dividend/<secid в нижнем регистре>` (HTML) |
| Telegram Bot API | notifier | `getMe`, `getUpdates` (long polling), `sendMessage` |
