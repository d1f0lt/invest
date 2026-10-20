# 4. Модель данных

Все сервисы используют одну базу PostgreSQL 16 `invest`. Каждая таблица
принадлежит ровно одному сервису: только он в неё пишет и только он хранит
её миграции. Другие сервисы могут читать чужие таблицы напрямую.  

## 4.1. Владение таблицами

| Сервис-владелец | Таблицы | Кто ещё читает |
|---|---|---|
| `price_updater` | `securities`, `latest_prices`, `candles`, `candle_history_sync`, `security_metadata` | `securities_reader`, `portfolio`, `notifier` |
| `users` | `users`, `refresh_tokens` | — |
| `portfolio` | `portfolios`, `portfolio_members`, `trades`, `cash_operations`, `brokers`, `report_imports` | — |
| `notifier` | `alerts`, `telegram_links`, `telegram_link_tokens` | — |
| инфраструктура | `schema_migrations` | `scripts/run-migrations.sh` |

`gateway`, `securities_reader` и `parser` своих таблиц не имеют.

## 4.2. Описание сущностей

### Рыночные данные (`price_updater`)

| Таблица | Назначение | Объём и время жизни |
|---|---|---|
| `securities` | справочник бумаг; ключ — пара `(secid, board)`, потому что у MOEX нет единого номера бумаги | несколько тысяч строк, upsert раз в минуту |
| `latest_prices` | последняя известная цена и дневные агрегаты; пустой `LAST` не затирает прошлую цену; `prev_close` — закрытие прошлого дня | одна строка на бумагу |
| `candles` (`10m`) | официальные 10-минутные свечи MOEX с объёмом и оборотом | последний успешно загруженный торговый день бумаги |
| `candles` (`1h`) | часовые свечи, собранные из поминутного опроса (без объёма) | только последний торговый день бумаги |
| `candles` (`1d`) | дневные свечи; днём — живые, ночью перезаписываются официальной историей | бессрочно |
| `security_metadata` | описание MOEX (`description`, JSONB), эмитент, дивиденды dohod.ru (JSONB); независимые `info_updated_at` и `dividends_updated_at` | одна строка на SECID; обновление описания раз в 12 ч, дивидендов раз в 24 ч; данные сохраняются при ошибках источника |
| `candle_history_sync` | до какой даты загружена официальная история по каждому board | по строке на board |

### Пользователи (`users`)

| Таблица | Назначение |
|---|---|
| `users` | аккаунт; уникальность email и логина без учёта регистра через функциональные индексы `LOWER(...)` |
| `refresh_tokens` | refresh-токены только в виде SHA-256; `revoked_at IS NULL` — токен действует; `family_id` связывает цепочку ротаций одной сессии; строки удаляются через 30 дней после истечения |

### Портфель (`portfolio`)

| Таблица | Назначение |
|---|---|
| `portfolios` | портфель пользователя; `user_id` без внешнего ключа на `users` (слабая связь между сервисами) |
| `portfolio_members` | состав составного портфеля; портфель составной, если у него есть строки здесь; вложенность запрещена |
| `trades` | **неизменяемый журнал** сделок; цена — в деньгах за одну бумагу без НКД; FK на `securities` |
| `cash_operations` | неизменяемый журнал денежных операций: `deposit`, `withdrawal`, `dividend`, `coupon`, `redemption`, `tax`, `fee`, `other`; `amount > 0` — пришло на счёт; без FK на `securities` |
| `brokers` | поддерживаемые брокеры и допустимые форматы файлов; засеяны `sber` (HTML) и `tbank` (XLSX, парсера пока нет) |
| `report_imports` | статус каждой загрузки отчёта; `id` совпадает с `task_id` задачи в очереди |

### Уведомления (`notifier`)

| Таблица | Назначение |
|---|---|
| `alerts` | ценовое уведомление; хранится всегда целевая цена, процент — только для отображения (`input_percent`); активное ⇔ `triggered_at IS NULL` |
| `telegram_links` | привязка «пользователь ↔ чат», по одному чату на пользователя и наоборот |
| `telegram_link_tokens` | одноразовые токены для `/start <token>`, только SHA-256, TTL 15 минут |

## 4.3. Миграции

- Лежат в `backend/services/<service>/migrations/NNNN_<name>.sql` и пишутся
  идемпотентно (`IF NOT EXISTS`, `ON CONFLICT DO NOTHING`, `DO $$ … $$`).
- Применяются скриптом `backend/scripts/run-migrations.sh`:
  - порядок сервисов задан префиксами точек монтирования: `10_price_updater`,
    `20_users`, `30_portfolio`, `40_notifier`. `price_updater` идёт первым,
    потому что на `securities` ссылаются `portfolio` и `notifier`;
  - каждый файл выполняется в одной транзакции вместе с записью в
    `schema_migrations (service, filename)`, поэтому повторный запуск
    применяет только новые файлы;
  - на пустой базе скрипт запускается автоматически при первом старте
    контейнера postgres (через `00-run-migrations.sql`). При выкатке его
    вызывает `deploy.sh`, вручную — `docker compose exec postgres sh /opt/invest/run-migrations.sh`.
- Миграции только накатываются, отката нет. Новая миграция обязана работать
  со **старой** версией кода, потому что откат выкатки миграции не
  откатывает.

Журнал миграций:

| Сервис | Файл | Изменение |
|---|---|---|
| price_updater | `0001_init.sql` | `securities`, `price_snapshots`, вьюха `latest_prices` |
| price_updater | `0002_bonds.sql` | `face_value`, `price_in_percent` |
| price_updater | `0003_candles.sql` | `latest_prices` становится таблицей, `candles`, `candle_history_sync`; перенос данных и удаление `price_snapshots` |
| price_updater | `0004_prev_close.sql` | `latest_prices.prev_close` |
| price_updater | [`0005_intraday_candles.sql`](../backend/services/price_updater/migrations/0005_intraday_candles.sql) | допустимый интервал свечей `10m` |
| price_updater | [`0006_security_metadata.sql`](../backend/services/price_updater/migrations/0006_security_metadata.sql) | таблица `security_metadata` |
| users | `0001_init.sql` | `users` |
| users | `0002_refresh_tokens.sql` | `refresh_tokens` |
| users | `0003_refresh_token_families.sql` | `family_id`, индекс по `expires_at` |
| portfolio | `0001_init.sql` | `portfolios`, `trades`, тип `trade_side` |
| portfolio | `0002_report_import.sql` | `accrued_interest`, `external_id`, `cash_operations` |
| portfolio | `0003_report_imports.sql` | `brokers` (+ `sber`), `report_imports` |
| portfolio | `0004_composite_portfolios.sql` | `portfolio_members` |
| portfolio | `0005_tbank_broker.sql` | брокер `tbank` как заглушка |
| notifier | `0001_init.sql` | `alerts`, `telegram_links`, `telegram_link_tokens` |
| notifier | `0002_input_percent.sql` | `alerts.input_percent` |

## 4.4. Объектное хранилище и очередь

| Ресурс | Содержимое | Ключ | Время жизни |
|---|---|---|---|
| S3-бакет `reports` | исходные файлы отчётов брокера (содержат персональные данные) | `{portfolio_id}/{task_id}_{filename}` | бессрочно, очистки нет (уточнить политику хранения) |
| Очередь `report.uploaded` | задачи разбора (JSON, ссылка на файл, не сам файл) | — | до `Ack`/`Nack` |
