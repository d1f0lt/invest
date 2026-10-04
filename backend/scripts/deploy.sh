#!/bin/sh

set -eu
cd "$(dirname "$0")/.."

docker compose pull --quiet

# поднимаем postgres отдельно, чтобы миграции могли выполняться в отдельном контейнере
# и ждем, пока он станет готов к подключению
docker compose up -d --no-build postgres
i=0
until docker compose exec -T postgres pg_isready -q -h 127.0.0.1 -U postgres; do
    i=$((i + 1))
    if [ "$i" -ge 60 ]; then
        echo "deploy.sh: postgres did not become ready" >&2
        exit 1
    fi
    sleep 2
done

# запуск миграций
docker compose exec -T postgres sh -s < scripts/run-migrations.sh

# запуск всего остального 
docker compose up -d --no-build --remove-orphans

# ждем, пока gateway станет здоровым
i=0
while :; do
    status=$(docker inspect -f '{{.State.Health.Status}}' "$(docker compose ps -q gateway)")
    [ "$status" = healthy ] && break
    i=$((i + 1))
    if [ "$i" -ge 36 ]; then
        echo "deploy.sh: gateway is $status after 3 minutes" >&2
        docker compose ps
        docker compose logs --tail 50 gateway
        exit 1
    fi
    sleep 5
done

docker image prune -af
docker compose ps
