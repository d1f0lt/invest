# Архитектурные решения (ADR)

Здесь записаны принятые решения, их причины и ограничения.
При пересмотре создавайте
новый ADR и связывайте его со старым.

| № | Решение |
|---|---|
| [0001](0001-microservices-shared-database.md) | Микросервисы на Go с общей базой PostgreSQL |
| [0002](0002-single-gateway-trust-model.md) | Единственная точка входа gateway и доверие к `x-user-id` внутри сети |
| [0003](0003-grpc-internal-communication.md) | gRPC для межсервисного взаимодействия |
| [0004](0004-proto-ownership.md) | `.proto` только у владельца, никаких общих Go-модулей |
| [0005](0005-async-report-import.md) | Асинхронный импорт отчётов через объектное хранилище и RabbitMQ |
| [0006](0006-trade-journal-weighted-average.md) | Журнал операций и метод средневзвешенной стоимости |
| [0007](0007-idempotent-import-external-id.md) | Идемпотентный импорт по `external_id` |
| [0008](0008-refresh-token-rotation.md) | Refresh-токены с ротацией, семьями и обнаружением повторного использования |
| [0009](0009-seaweedfs-instead-of-minio.md) | SeaweedFS вместо MinIO |
| [0010](0010-deploy-compose-single-server.md) | Развёртывание: один сервер, Docker Compose, выкатка по тегу |
| [0011](0011-composite-portfolios.md) | Составные портфели: расчёт по участникам и слияние |
| [0012](0012-telegram-long-polling.md) | Уведомления о цене: отдельный notifier, опрос БД и Telegram |
| [0013](0013-flutter-mobile-client.md) | Мобильный клиент на Flutter |
| [0014](0014-candles-from-database.md) | Графики читаются из БД, свечи загружает price_updater |
| [0015](0015-reader-database-only.md) | securities_reader читает только БД |
