# parser

Микросервис на Go: асинхронно разбирает загруженные пользователем отчёты
брокера и заносит найденные сделки в `portfolio`.

**Это заготовка.** Реализовано всё, кроме самих парсеров форматов:
подписка на очередь RabbitMQ, скачивание файла из MinIO, диспетчер
`map[broker]parsing.Parser` (выбор парсера по имени брокера из задачи),
отправка распознанных сделок в `portfolio` одним пакетным gRPC-вызовом
`CreateTrades`, health-check, graceful shutdown. Зарегистрирован один
парсер — `parsing.Stub` под брокером `tinkoff`, который для любого
входа возвращает `parsing.ErrNotImplemented`. Задача при этом не
теряется молча: воркер логирует и снимает её с очереди (см. ниже, почему
без requeue), но результат в `portfolio` не попадает, пока `Stub` не
заменят на реальный парсер.

## Поток данных

Решено в отдельной сессии обсуждения архитектуры (см.
`architecture-decisions.md` в корне репозитория, раздел "parser:
асинхронный разбор отчётов"):

```
пользователь → gateway → сохраняет файл в MinIO
                        → публикует задачу в RabbitMQ (очередь "report.uploaded")
                                                            │
                                                            ▼
                                                          parser
                                          скачивает файл из MinIO по task.ObjectKey
                                          выбирает парсер по task.Broker (map[broker]Parser)
                                          разбирает (Stub — не реализовано)
                                          CreateTrades (gRPC, вся пачка одним вызовом) → portfolio
```

`gateway` (`backend/services/gateway`) реализован и в обычной работе
публикует эту задачу сам после `POST /api/v1/portfolios/{id}/reports`
(с обязательным полем `broker` в multipart-форме — см. ниже).
Раздел "Ручной прогон" ниже по-прежнему полезен, чтобы гонять `parser`
в изоляции, без поднятого `gateway`/`users`/`portfolio`.

## Формат задачи (`internal/task.ReportUploaded`)

JSON, одно сообщение — одна задача, публикуется в очередь
`report.uploaded` через default exchange (routing key = имя очереди) —
не изменилось переходом на gRPC, это по-прежнему сообщение RabbitMQ, а
не вызов какого-либо API:

```json
{
  "task_id": "b6e6c6b0-...-uuid",
  "user_id": "3fa2...-uuid пользователя, которому принадлежит portfolio_id",
  "portfolio_id": "9c11...-uuid портфеля, куда добавлять сделки",
  "broker": "tinkoff",
  "bucket": "reports",
  "object_key": "reports/<user_id>/<task_id>.pdf",
  "filename": "broker_report_2026-09.pdf",
  "content_type": "application/pdf",
  "uploaded_at": "2026-09-16T12:00:00Z"
}
```

`broker` (добавлено 2026-09-23) — имя брокера отчёта, которое
пользователь указывает при загрузке в `gateway` (поле `broker` в
multipart-форме; пропуск → 400). `gateway` нормализует его (lowercase,
пробелы → `-`) до публикации; `parser` применяет ту же нормализацию при
поиске по реестру, так что ключ задачи и ключ регистрации сходятся
независимо от регистра/пробелов. Парсер для не зарегистрированного
брокера → `parsing.ErrUnsupportedBroker` → задача снимается с очереди
(drop, не requeue — повтор не сделает неизвестный брокер известным).

`portfolio_id` в задаче — решение, принятое при написании этого
скелета, а не в обсуждении архитектуры (там до этого уровня детализации
не дошли): без него `parser` не знает, в какой из портфелей пользователя
класть сделки. Публикующая сторона (`gateway`) должна сама проверить, что
`portfolio_id` принадлежит `user_id`, — `parser` эту проверку не
повторяет (у него нет для этого механизма: `portfolio` сейчас доверяет
`x-user-id` без криптографической проверки, см. ниже).

`task_id` пока ни на что не влияет — заложен на будущее, для дедупликации
и таблицы статусов (`pending/processing/done/failed`), которую
обсуждали, но не спроектировали (см. architecture-decisions.md).

## Диспетчер парсеров (`internal/parsing.Dispatcher`)

Реестр `map[broker]parsing.Parser`. Новый брокер = реализация интерфейса
`parsing.Parser` (`Parse(t task.ReportUploaded, data []byte) ([]Trade,
error)`) + строка `parsers.Register("<broker>", …)` в
`cmd/parser/main.go`. Ключи нормализуются так же, как `gateway`
нормализует пользовательский ввод (`normalizeBroker`: lowercase, пробелы
→ `-`), поэтому регистрация под `"TCS Investments"` и задача с
`"tcs investments"` сходятся к одному ключу `tcs-investments`. Пока в
реестре один ключ: `tinkoff` → `parsing.Stub`.

## Как parser попадает в portfolio без логина пользователя

`parser` работает по очереди, без пользовательской сессии, и просто
прикрепляет `task.UserID` как gRPC-metadata `x-user-id` к каждому вызову
`portfolio` (см. `internal/authmd.WithUserID` и
`internal/portfolioclient.Client.SubmitTrades`) — `portfolio` сейчас
доверяет этому значению без проверки, см. его README.

**Изменилось 2026-09-20**: раньше это был HTTP-заголовок `X-User-ID` на
каждом `POST /portfolios/{id}/trades`; переведено на gRPC вместе с
`users`/`portfolio`/`price_reader` (см. architecture-decisions.md,
"перевод внутреннего взаимодействия сервисов на gRPC"). Модель доверия
не изменилась — то же значение, тот же уровень (без) проверки, просто
другой транспорт для его передачи.

**До 2026-09-18** `portfolio` требовал `Authorization: Bearer <JWT>`, и
`parser` знал тот же `JWT_SECRET`, что и `users`/`portfolio`, и сам
подписывал себе короткоживущий токен с `sub = task.UserID`. Проверку JWT
убрали из всех бэкенд-сервисов разом в пользу модели доверия внутри
закрытой docker-compose-сети: наружу не смотрит ни один порт, поэтому
единственный, кто может вызвать `portfolio`, — другой контейнер этой же
сети. Эту проверку берёт на себя `gateway` — единственная точка входа
снаружи. В `parser` по-прежнему нет зависимости от JWT: не нужен ни
`JWT_SECRET`, ни собственный `internal/auth`-пакет.

Явный риск этой модели: любой контейнер внутри сети может выдать себя
за любого пользователя, просто поставив нужный `x-user-id`. Решение
осознанно принято пользователем при условии, что "левых" контейнеров в
сети нет — см. architecture-decisions.md.

## Обработка ошибок / очередь

- `Qos(prefetch=1)` — один воркер обрабатывает одну задачу за раз
  (осознанный выбор в пользу RabbitMQ вместо Kafka, см.
  architecture-decisions.md).
- Битое сообщение (не парсится как JSON) или ошибка самого разбора файла
  (`parsing.ErrNotImplemented` и любая другая) → `Nack(requeue=false)`:
  повтор тут не поможет, а бесконечный retry-loop — не то, что нужно.
  Такие сообщения либо настройте с dead-letter exchange в RabbitMQ (не
  сделано в этой заготовке), либо они просто теряются — TODO на будущее.
- Ошибка скачивания из MinIO или отправки в `portfolio` (сеть,
  недоступность, любой gRPC-код ошибки) → `Nack(requeue=true)`: это
  может быть временным.

## gRPC-клиент к portfolio

`internal/portfoliopb` содержит только сгенерированный код
(`*.pb.go`/`*_grpc.pb.go`), никакого `.proto`-файла в этом каталоге на
диске постоянно не лежит. Канонический источник —
`../portfolio/proto/portfolio.proto` (`parser` — Go-клиент `portfolio`
по gRPC, а не совладелец общего пакета с ним; см.
`architecture-decisions.md`, "перевод внутреннего взаимодействия сервисов
на gRPC"). Перегенерировать после правки `portfolio.proto`:

```bash
make proto
```

Цель сама копирует `../portfolio/proto/portfolio.proto` сюда в
`internal/portfoliopb/`, гоняет `protoc` и удаляет скопированный
`.proto` — на диске остаются только сгенерированные файлы (см.
`Makefile` в этой директории; та же схема, что и у `gateway`).

```bash
make clean
```

Удаляет `internal/portfoliopb` целиком — после этого `go build`/`docker
build` не соберутся, пока не прогнать `make proto` заново (Docker-сборка
сама `make proto` не вызывает — см. `Dockerfile`). Нужен в основном для
проверки, что `make proto` восстанавливает код с нуля.

`internal/portfolioclient` — тонкая обёртка над
`portfoliopb.PortfolioServiceClient`: один `grpc.ClientConn`, открытый
один раз при старте процесса (не на каждую пачку — было бы избыточно;
gRPC-соединение и так мультиплексирует вызовы), и `SubmitTrades`, которая
**с 2026-09-23 отправляет всю пачку одним вызовом `CreateTrades`** (новая
пакетная RPC `portfolio`, вставляет все сделки в одной транзакции БД —
all-or-nothing). До этого сделки отправлялись по одной (`CreateTrade` на
каждую), и ошибка на середине пачки при `Nack(requeue=true)` приводила к
дубликатам уже принятых сделок при повторе задачи — эта проблема
исчезла вместе с посылкой. Таймаут на вызов один на всю пачку: базовые
30 секунд плюс 2 секунды на сделку. Дедупликация самих сделок
(например, при повторной публикации задачи) по-прежнему не реализована —
остаётся TODO.

## Health check

Собственного API у `parser` нет — весь смысл сервиса в фоновом
потреблении очереди. `internal/health` поднимает только стандартный gRPC
health-checking протокол (`grpc.health.v1.Health/Check`), проверяющий
RabbitMQ и MinIO на каждый вызов — прямая замена старому `GET /healthz`
(`internal/httpapi`, удалён). В отличие от старого HTTP-обработчика,
gRPC-ответ не содержит тела с деталями (`HealthCheckResponse` — это
только enum-статус), поэтому какая именно зависимость не отвечает — по-
прежнему видно, но уже в логах сервиса (`OnUnhealthy` в
`cmd/parser/main.go`), а не в теле ответа проверки.

```bash
grpcurl -plaintext localhost:8084 grpc.health.v1.Health/Check
```

## Конфигурация

См. `.env.example`. Ключевое:

| Переменная | Назначение |
|---|---|
| `RABBITMQ_URL`, `RABBITMQ_QUEUE` | подключение к очереди задач |
| `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_USE_SSL` | подключение к хранилищу файлов отчётов |
| `PORTFOLIO_GRPC_ADDR` | куда отправлять распознанные сделки (parser представляется через `x-user-id` metadata, без JWT — см. выше); было `PORTFOLIO_BASE_URL` (http://...) до перехода на gRPC |
| `GRPC_ADDR` | адрес gRPC health-сервера (проверяет RabbitMQ + MinIO); было `HTTP_ADDR` |

## Ручной прогон (в обход `gateway`, для отладки `parser` в изоляции)

```bash
docker compose up --build minio rabbitmq portfolio parser
```

1. Положить файл в MinIO (bucket `reports` создаётся автоматически
   сервисом `minio-init`, см. корневой `docker-compose.yml`):
   ```bash
   docker compose exec minio-init mc cp /etc/hostname local/reports/test.pdf
   # или через веб-консоль MinIO: http://localhost:9001 (minio / minio12345)
   ```
2. Создать портфель и получить его id (`users`/`portfolio` уже подняты
   отдельно, см. корневой README и `portfolio`'s README про `grpcurl`) —
   нужны `user_id` и `portfolio_id`.
3. Опубликовать задачу в очередь `report.uploaded`, например через
   management UI RabbitMQ (`http://localhost:15672`, `invest` /
   `invest12345`, вкладка очереди → Publish message) с телом из раздела
   "Формат задачи" выше (не забыть поле `broker` — без него задача
   упадёт с "unsupported broker").
4. `docker compose logs -f parser` — должно быть видно, что задача
   забрана, файл скачан из MinIO, и `parsing not implemented yet,
   dropping task` (для `broker: "tinkoff"`). Для незарегистрированного
   брокера (например, `"sber"`) — `unsupported broker, dropping task`.
   Это ожидаемо: реальные парсеры ещё не реализованы.

## Что дальше (не входит в эту заготовку)

- Реализовать `parsing.Parser` для конкретных брокеров — по одному на
  формат отчёта, регистрация в `cmd/parser/main.go` через
  `Dispatcher.Register` (выбор парсера уже не по `ContentType`, а по
  полю `broker` задачи — `ContentType` различает PDF/CSV/XLSX, но не
  брокера).
- Таблица/эндпоинт статуса задачи для поллинга клиентом.
- Dead-letter очередь для сообщений, которые `Nack(requeue=false)`.
- Дедупликация по `task_id` — `gateway` не переопубликовывает задачу
  сам, но RabbitMQ в целом даёт only at-least-once доставку, так что
  повтор в принципе возможен (см. также раздел про gRPC-клиент выше:
  благодаря пакетной `CreateTrades` повтор теперь не оставит частично
  применённую пачку, но дубликаты всей пачки всё ещё возможны).
