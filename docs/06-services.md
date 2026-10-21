# 6. Каталог сервисов

## Сводная таблица

| Сервис | Тип | Порт (внутри сети) | Хранилища | Вызывает | Вызывается из |
|---|---|---|---|---|---|
| [gateway](#gateway) | HTTP API | 8080 | S3, RabbitMQ | users, portfolio, securities_reader, notifier | приложение (через Caddy) |
| [users](#users) | gRPC | 8082 | PostgreSQL | — | gateway |
| [portfolio](#portfolio) | gRPC | 8083 | PostgreSQL | — | gateway, parser |
| [securities_reader](#securities_reader) | gRPC | 8081 | PostgreSQL (чтение) | — | gateway |
| [price_updater](#price_updater) | фоновый | — | PostgreSQL | MOEX ISS, dohod.ru | — |
| [parser](#parser) | воркер очереди | 8084 (health) | S3, RabbitMQ | portfolio | RabbitMQ |
| [notifier](#notifier) | gRPC + бот | 8085 | PostgreSQL | Telegram Bot API | gateway |

Все сервисы: Go 1.24, JSON-логи
в stdout, graceful shutdown, gRPC reflection включён (кроме gateway).

---

## gateway

**Ответственность:** единственная точка входа. Предоставляет REST/JSON API
мобильному клиенту, проверяет JWT, переводит HTTP-запросы в gRPC-вызовы,
принимает файлы отчётов, отдаёт иконки брокеров.

| | |
|---|---|
| Код | `backend/services/gateway` (точка входа — `cmd/main.go`) |
| Внешний API | [REST API](api/openapi.md), [OpenAPI](api/openapi.yaml) |
| Зависимости при старте | users, portfolio, securities_reader, notifier (healthy), RabbitMQ (healthy), s3-init (завершён) |
| Health | `GET /healthz` — 200 или 503 с перечнем проблем 
| Исходники | [gateway](../backend/services/gateway/) |

## users

**Ответственность:** аккаунты, хранение паролей, выпуск и ротация токенов,
профиль.

| | |
|---|---|
| gRPC | `invest.users.v1.UsersService`: `Register`, `Login`, `RefreshToken`, `Logout`, `GetMe`, `GetUser`, `UpdateMe`, `ChangePassword`, `DeleteMe` |
| Таблицы | `users`, `refresh_tokens` |
| Фоновые задачи | раз в час удаляет refresh-токены, истёкшие больше 30 дней назад |
| Исходники | [users](../backend/services/users/) |

## portfolio

**Ответственность:** портфели (обычные и составные), журнал сделок и денежных
операций, расчёт позиций, P&L, стоимости по дням, справочник брокеров, статусы
импорта отчётов.

| | |
|---|---|
| gRPC | `invest.portfolio.v1.PortfolioService`: 17 RPC, см. [внутренние контракты](api/internal-contracts.md#portfolio) |
| Таблицы | `portfolios`, `portfolio_members`, `trades`, `cash_operations`, `brokers`, `report_imports`; читает `securities`, `latest_prices`, `candles` |
| Особенности | смена площадки торгов бумаги (например, TQTF → TQBR) учитывается при расчёте, данные в БД не меняются; вводные остатки (`opening`) из отчёта применяются, только если по счёту нет более ранних строк |
| Исходники | [portfolio](../backend/services/portfolio/) |

Формула итоговой прибыли портфеля:

```
total_pnl = realized + unrealized
          + total_dividends + total_coupons + total_accrued_interest
          + total_taxes + total_fees + total_other
```

Контрольное тождество: `рыночная стоимость + cash_balance − net_deposits = total_pnl`.

## securities_reader

**Ответственность:** API чтения рыночных данных (бывший `price_reader`,
переименован 2026-09-25).

| | |
|---|---|
| gRPC | `invest.securitiesreader.v1.SecuritiesReaderService`: `GetPrices`, `GetCandles`, `SearchSecurities`, `GetSecurityInfo`, `GetDividends` |
| Таблицы (чтение) | `securities`, `latest_prices`, `candles`, `security_metadata` |
| Поиск | один SQL-запрос по тикеру, названию и ISIN с ранжированием, `ё` = `е`; без индексов (бумаг несколько тысяч) |
| Исходники | [securities_reader](../backend/services/securities_reader/) |

## price_updater

**Ответственность:** держать в БД цены, свечи, описания бумаг MOEX и дивиденды dohod.ru.

| | |
|---|---|
| Режимы торгов | `TQBR` (акции), `TQTF` (фонды), `TQOB` (ОФЗ), `TQCB` (корпоративные облигации) |
| Поминутная задача | справочник, `latest_prices`, живые часовые и дневные свечи |
| Отдельная задача свечей | официальные 10-минутные свечи последнего торгового дня; проход при старте и через `POLL_INTERVAL` после завершения, пауза между бумагами `HISTORY_REQUEST_PAUSE` |
| Ночная задача | 03:00 МСК и при старте: официальная дневная история с последней загруженной даты (первый запуск — за 365 дней), чистка часовых свечей |
| Метаданные и дивиденды | независимый проход при старте и через 15 мин после завершения; описание [MOEX](../backend/services/price_updater/internal/moex/) раз в 12 ч, дивиденды [dohod.ru](../backend/services/price_updater/internal/dohod/) раз в 24 ч |
| Исходники | [price_updater](../backend/services/price_updater/) |

## parser

**Ответственность:** асинхронный разбор отчётов брокера и передача всех
операций в `portfolio`.

| | |
|---|---|
| Вход | очередь RabbitMQ `report.uploaded`, `prefetch=1` |
| Выход | `portfolio.ImportReport` (один вызов на отчёт, таймаут 30 с + 200 мс на строку), `UpdateReportImportStatus` |
| Поддерживаемые брокеры | `sber` — HTML «Отчёт брокера» СберИнвестиций. `tbank` есть в справочнике брокеров, но парсера нет |
| Расширение | новый брокер = реализация `parsing.Parser` + `Register` в `cmd/main.go` + строка в `portfolio.brokers` |
| README | [backend/services/parser/README.md](../backend/services/parser/README.md) |

## notifier

**Ответственность:** ценовые уведомления и их доставка в Telegram.

| | |
|---|---|
| gRPC | `invest.notifier.v1.NotifierService`: `CreateAlert`, `ListAlerts`, `UpdateAlert`, `DeleteAlert`, `CreateTelegramLink`, `GetTelegramLink`, `DeleteTelegramLink` |
| Таблицы | `alerts`, `telegram_links`, `telegram_link_tokens`; читает `latest_prices`, `securities` |
| Фоновые задачи | проверка алертов раз в минуту; Telegram-бот на long polling (`/start <token>`, `/stop`) |
| Ограничения | не больше 100 алертов на пользователя; **только один экземпляр** на токен бота (иначе Telegram вернёт 409 на `getUpdates`); без `TELEGRAM_BOT_TOKEN` сервис работает, но ничего не отправляет |
| Исходники | [notifier](../backend/services/notifier/) |

---

## Инфраструктурные компоненты

| Компонент | Образ | Назначение | Данные |
|---|---|---|---|
| postgres | `postgres:16-alpine` | общая БД `invest` | volume `postgres_data` |
| s3 | `chrislusf/seaweedfs:4.06` | S3-совместимое хранилище (master + volume + filer + S3 в одном контейнере) | volume `s3_data` |
| s3-init | `chrislusf/seaweedfs:4.06` | одноразово создаёт бакет `reports` | — |
| rabbitmq | `rabbitmq:3-management-alpine` | очередь задач, management UI | без volume: durable-очередь и persistent-сообщения переживают рестарт, но не пересоздание контейнера |
| caddy | `caddy:2-alpine` | HTTPS-прокси, только профиль `https` | volumes `caddy_data`, `caddy_config` (сертификаты) |
