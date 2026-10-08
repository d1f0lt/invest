# Внешний REST API (gateway)

Контракт API описан в [openapi.yaml](openapi.yaml).

Для просмотра интерактивной документации запустите Swagger UI из корня проекта:

```bash
docker run --rm -p 8081:8080 \
  -e SWAGGER_JSON=/spec/openapi.yaml \
  -v "$PWD/docs/api:/spec:ro" \
  swaggerapi/swagger-ui
```

Откройте [http://localhost:8081](http://localhost:8081).
