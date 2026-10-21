# 5. Безопасность

## 5.1. Периметр

- Из интернета доступны только Caddy (`80` для выпуска сертификата, `8443`
  для API).
- `gateway` на сервере слушает только `127.0.0.1:8080` (`GATEWAY_BIND`).
- PostgreSQL, S3 и RabbitMQ публикуются только на `127.0.0.1`. Docker
  обходит правила `ufw`, поэтому порты без привязки к `127.0.0.1` были бы
  открыты в интернет.
- Внутренние сервисы портов не публикуют вообще.

## 5.2. Аутентификация

| Механизм | Реализация |
|---|---|
| Пароли | bcrypt (`golang.org/x/crypto/bcrypt`), cost 12, соль внутри хеша; минимум 8 символов |
| Access-токен | JWT HS256, `sub` = id пользователя, TTL 30 минут, секрет `JWT_SECRET` (общий у `users` и `gateway`); gateway принимает только `HS256` (`WithValidMethods`) — защита от `alg: none` и algorithm confusion |
| Refresh-токен | 32 случайных байта, TTL 30 дней, в БД — только SHA-256 |
| Ротация | каждый `refresh` отзывает старый токен атомарным `UPDATE … WHERE revoked_at IS NULL AND expires_at > now()` |
| Обнаружение повторного использования | токен, отозванный больше 30 с назад, считается украденным: отзывается вся семья (`family_id`) |
| Смена пароля | отзываются все refresh-токены аккаунта, вызывающему выдаётся новая пара |
| Перебор email | одинаковые ответ и время для «нет такого email» и «неверный пароль» |
| Хранение на клиенте | `flutter_secure_storage` (Keychain / Android Keystore) |

## 5.3. Авторизация и модель доверия

- Публичные маршруты: `POST /api/v1/users`, `/login`, `/refresh`, `/logout`,
  `GET /healthz`, `GET /static/brokers/{file}`. Все остальные требуют
  `Authorization: Bearer <JWT>`.
- `gateway` — **единственный** компонент, который проверяет JWT. После
  проверки он сам добавляет к исходящему gRPC-вызову metadata `x-user-id`.
  HTTP-заголовки клиента во внутренние сервисы не пробрасываются вообще.
- Внутренние сервисы доверяют `x-user-id` без проверки. Это безопасно, пока
  выполняется допущение: **в docker-сети нет посторонних контейнеров**
  ([ADR-0002](adr/0002-single-gateway-trust-model.md)).
- Доступ к данным ограничен владельцем: `portfolio` и `notifier` фильтруют
  все запросы по `x-user-id`, а чужой ресурс отдают как `NotFound`.
- Ролей и администраторских API нет.

## 5.4. Секреты

| Секрет | Где хранится | Кто использует |
|---|---|---|
| `JWT_SECRET` | `backend/.env` (сервер) / секрет `ENV_FILE` в GitHub | `users`, `gateway` |
| `POSTGRES_PASSWORD`, `S3_ACCESS_KEY`/`S3_SECRET_KEY`, `RABBITMQ_USER`/`RABBITMQ_PASSWORD` | там же | инфраструктура и сервисы |
| `TELEGRAM_BOT_TOKEN` | там же | `notifier` |
| `DEPLOY_SSH_KEY`, `DEPLOY_KNOWN_HOSTS`, `DEPLOY_HOST`, `DEPLOY_USER` | GitHub, environment `production` | workflow `deploy` |
| `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD` | GitHub secrets | workflow `mobile-release` |

## 5.5. Защита данных

- Пароли и refresh-токены нигде не хранятся и не логируются в открытом виде.
- Отчёты брокера содержат ФИО, номер договора и операции по счёту. Они лежат
  в S3 бессрочно, без шифрования на уровне приложения
