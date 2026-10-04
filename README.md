# invest

## Деплой

Backend выкатывается на сервер тегом `v*`:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Workflow [`deploy`](.github/workflows/deploy.yml):

1. прогоняет CI backend (тесты, миграции);
2. собирает 7 образов и пушит их в `ghcr.io/d1f0lt/invest/<сервис>:<тег>`;
3. копирует на сервер в `/opt/invest` файлы `docker-compose.yml`, `caddy/`, `scripts/`, миграции и `.env` (из секрета `ENV_FILE`);
4. запускает там [`backend/scripts/deploy.sh`](backend/scripts/deploy.sh): `pull` → миграции → `up -d` → ждёт healthy gateway → чистит старые образы;
5. проверяет `https://d1f0lt-invest.duckdns.org:8443/healthz`.

**Откат / повторная выкатка:** Actions → deploy → Run workflow → указать уже собранный тег. Образы не пересобираются, миграции не откатываются — поэтому миграция должна работать и со старой версией кода.

**HTTPS:** на сервере перед gateway стоит Caddy (`backend/caddy/Caddyfile`, профиль `https`). Сертификат Let's Encrypt он получает сам через порт 80, HTTPS отдаёт на **8443** — порт 443 на сервере занят VPN. Адрес API: `https://d1f0lt-invest.duckdns.org:8443`. Порт gateway 8080 на сервере слушает только `127.0.0.1`.

**Секреты** (environment `production`): `DEPLOY_HOST`, `DEPLOY_USER`, `DEPLOY_SSH_KEY`, `DEPLOY_KNOWN_HOSTS`, `ENV_FILE` (содержимое `.env` по образцу `backend/.env.example`).
