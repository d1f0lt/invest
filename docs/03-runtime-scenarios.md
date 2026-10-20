# 3. Сценарии выполнения

## 3.1. Регистрация и вход

```mermaid
sequenceDiagram
    autonumber
    actor U as Пользователь
    participant A as Приложение
    participant G as gateway
    participant US as users
    participant DB as PostgreSQL

    U->>A: email, пароль
    A->>G: POST /api/v1/users
    G->>US: Register
    US->>DB: INSERT users (bcrypt-хеш, cost 12)
    US-->>G: User
    G-->>A: 201 User

    A->>G: POST /api/v1/login
    G->>US: Login
    US->>DB: SELECT по LOWER(email)
    US->>US: bcrypt.Compare (при неизвестном email — сравнение с фиктивным хешем)
    US->>DB: INSERT refresh_tokens (SHA-256, новая family_id)
    US-->>G: access_token (JWT, 30 мин), refresh_token (30 дней)
    G-->>A: 200 LoginResponse
    A->>A: сохранить пару в Keychain / Android Keystore
```

- Неизвестный email и неверный пароль дают одинаковую ошибку
  `invalid email or password` с одинаковым временем ответа: защита от
  перебора email (user enumeration).

## 3.2. Авторизованный запрос и обновление токена

```mermaid
sequenceDiagram
    autonumber
    participant A as Приложение (Session)
    participant G as gateway
    participant US as users
    participant S as Любой сервис

    A->>A: access_token истекает менее чем через 60 с?
    alt да
        A->>G: POST /api/v1/refresh {refresh_token}
        G->>US: RefreshToken
        US->>US: UPDATE … SET revoked_at=now() WHERE token_hash=… AND revoked_at IS NULL AND expires_at>now()
        alt токен отозван > 30 с назад (повторное использование)
            US->>US: отозвать всю family_id
            US-->>A: 401 → экран входа
        else ок
            US-->>A: новая пара (та же family_id)
        end
    end
    A->>G: GET /api/v1/… Authorization: Bearer JWT
    G->>G: проверить HS256-подпись и срок, извлечь sub
    G->>S: gRPC + metadata x-user-id=sub
    S-->>G: ответ или gRPC-код
    G-->>A: JSON или HTTP-статус
    opt ответ 401
        A->>A: один раз обновить пару и повторить запрос
    end
```

## 3.3. Загрузка и разбор отчёта брокера

```mermaid
sequenceDiagram
    autonumber
    participant A as Приложение
    participant G as gateway
    participant PF as portfolio
    participant S3 as SeaweedFS (S3)
    participant MQ as RabbitMQ
    participant P as parser

    A->>G: POST /portfolios/{id}/reports (multipart: file, broker)
    G->>G: task_id = UUID, broker → lowercase, пробелы → "-"
    G->>PF: CreateReportImport(id=task_id, …)
    Note right of PF: проверка владения портфелем,<br/>брокер включён, расширение из file_formats,<br/>портфель не составной
    PF-->>G: ReportImport{status: queued}
    G->>S3: PUT reports/{portfolio_id}/{task_id}_{filename}
    alt ошибка S3
        G->>PF: UpdateReportImportStatus(failed)
        G-->>A: 502
    end
    G->>MQ: publish report.uploaded (JSON-задача)
    alt ошибка публикации
        G->>S3: DELETE объект (компенсация)
        G->>PF: UpdateReportImportStatus(failed)
        G-->>A: 502
    end
    G-->>A: 202 ReportImport{queued}

    loop каждые ~1,5 с
        A->>G: GET /portfolios/{id}/reports/{task_id}
        G->>PF: GetReportImport
        PF-->>A: queued / processing / done / failed
    end

    MQ->>P: доставка (prefetch=1)
    P->>PF: UpdateReportImportStatus(processing)
    P->>S3: GET объект
    P->>P: Dispatcher[broker].Parse → сделки + денежные операции
    P->>PF: ImportReport(report_import_id, trades, cash_operations, account_key, period_start)
    Note right of PF: одна транзакция:<br/>INSERT … ON CONFLICT (portfolio_id, external_id) DO NOTHING,<br/>статус done + счётчики
    PF-->>P: создано / пропущено
    P->>MQ: Ack
```

Обработка ошибок в `parser`:

| Ситуация | Действие с сообщением | Статус импорта |
|---|---|---|
| битый JSON задачи | `Nack(requeue=false)` | не меняется |
| брокер не поддерживается | `Nack(requeue=false)` | `failed` («Отчёты этого брокера пока не поддерживаются») |
| файл не разобрался | `Nack(requeue=false)` | `failed` с текстом для пользователя |
| `portfolio` окончательно отклонил (`InvalidArgument`, `FailedPrecondition`, …) | `Nack(requeue=false)` | `failed`, для неизвестной бумаги — с её `SECID/BOARD` |
| S3 или `portfolio` временно недоступны | `Nack(requeue=true)` | остаётся `processing` |
| пустой отчёт | `Ack` | `done` с нулями |

Dead-letter очереди нет: отклонённые без повтора сообщения теряются (статус
импорта при этом уже `failed`).

## 3.4. Обновление цен (price_updater)

```mermaid
sequenceDiagram
    autonumber
    participant T as Таймер (POLL_INTERVAL = 1 мин)
    participant PU as price_updater
    participant M as MOEX ISS
    participant DB as PostgreSQL

    loop для каждого board: TQBR, TQTF, TQOB, TQCB
        T->>PU: тик
        PU->>M: GET /engines/stock/markets/{shares|bonds}/boards/{BOARD}/securities.json
        M-->>PU: справочник + marketdata
        PU->>DB: upsert securities
        PU->>DB: COPY во временную таблицу → upsert latest_prices
        opt TRADINGSTATUS = 'T'
            PU->>DB: upsert часовой свечи текущего часа (МСК)
            PU->>DB: upsert дневной свечи из дневных агрегатов MOEX
        end
    end

    Note over PU: при старте и ежедневно в HISTORY_RUN_AT (03:00 МСК)
    PU->>DB: SELECT synced_through FROM candle_history_sync
    loop каждая дата после synced_through по вчера
        PU->>M: GET /history/…/securities.json?date=… (страницы по 100)
        PU->>DB: перезапись дневных свечей официальными
    end
    PU->>DB: удалить часовые свечи, кроме последнего торгового дня бумаги
```

Отдельная задача `price_updater` выбирает из БД последний день с дневной
свечой для каждой бумаги на `MOEX_BOARDS` и загружает из MOEX её
10-минутные свечи (`interval=10`, все страницы). Сохраняет OHLC, объём и оборот
через upsert с заменой, включая текущую незавершённую свечу.

## 3.5. Свечи для графика (securities_reader)

`securities_reader` получает все свечи из PostgreSQL. Запрос графика не
вызывает MOEX. При сбое загрузчика остаются последние сохранённые данные.

| `range` в API | Данные в БД | Свеча |
|---|---|---|
| `day` | 10-минутные свечи последнего доступного торгового дня; если они ещё не загружены для нового дня — часовые свечи из поминутного опроса | `10min` / `hour` |
| `week`, `month`, `year` | дневные свечи за выбранный период | `day` |
| `5y` | недельная агрегация дневных свечей, ограниченная последними 5 годами | `week` |
| `all` | недельная агрегация всей сохранённой дневной истории | `week` |

Глубина доступной истории зависит от `BACKFILL_DAYS` и прогресса загрузки.
По умолчанию это 365 дней: период `5y` не создаёт отсутствующие данные.

### Описание бумаги и дивиденды

`price_updater` запускает независимую задачу при старте и через 15 минут
после завершения прохода. Она выбирает бумаги на `MOEX_BOARDS`, для которых
описание старше 12 часов или дивиденды старше 24 часов (либо ещё не загружены).
Между бумагами — `HISTORY_REQUEST_PAUSE`.

Описание и эмитент загружаются из MOEX и сохраняются вместе; дивиденды —
из dohod.ru отдельным обновлением. Ошибка одного источника не мешает другому
и не затирает сохранённые данные. На облигации запросы дивидендов не делаются.

`securities_reader` читает `security_metadata` при каждом запросе, без
внешних вызовов и кэша. Пока описание не загружено, карточка содержит
название, ISIN и размер лота из `securities`. Пока дивиденды не загружены,
возвращается `Unavailable` (HTTP 502); успешно загруженный пустой список —
обычный ответ. При последующих сбоях источника выдаются прежние данные.

## 3.6. Ценовое уведомление

```mermaid
sequenceDiagram
    autonumber
    participant A as Приложение
    participant G as gateway
    participant N as notifier
    participant DB as PostgreSQL
    participant TG as Telegram

    A->>G: POST /api/v1/telegram/link
    G->>N: CreateTelegramLink
    N->>DB: INSERT telegram_link_tokens (SHA-256, TTL 15 мин)
    N-->>A: url = https://t.me/<bot>?start=<token>
    A->>TG: открыть ссылку, пользователь жмёт Start
    N->>TG: getUpdates (long polling)
    TG-->>N: /start <token>
    N->>DB: telegram_links(user_id ↔ chat_id)

    A->>G: POST /api/v1/alerts {secid, change_percent: -5}
    G->>N: CreateAlert
    N->>DB: base_price = latest_prices.last_price, target = base × 0,95, округлить до шага цены
    N-->>A: 201 Alert{direction: below, status: active}

    loop CHECK_INTERVAL = 1 мин
        N->>DB: активные алерты ⨝ latest_prices (trading_status='T', свежее 5 мин)
        alt цена пересекла цель
            N->>TG: sendMessage
            N->>DB: status = triggered (если updated_at не изменился)
        end
    end
```

- Уведомление срабатывает один раз. Повторно оно проверяется только после
  `PUT /api/v1/alerts/{id}`.
- Если Telegram вернул 403 или `chat not found`, привязка удаляется.
  При 429 оставшиеся уведомления переносятся на следующую проверку.
