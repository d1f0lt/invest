# --- build stage ---
FROM golang:1.24-alpine AS build
WORKDIR /src

# Build context is this service's own directory again (docker-compose.yml
# points context/dockerfile back here) - the generated gRPC/protobuf code
# used to live in a sibling module (backend/proto), which forced the
# context out to the repo root; it's now this service's own top-level
# proto/ package (proto/pricereader.proto + generated *.pb.go, both
# committed), so there's nothing outside this directory to reach for any
# more. See architecture-decisions.md, "перевод внутреннего
# взаимодействия сервисов на gRPC" for why the shared module was undone.
# `COPY . .` below just picks up the already-generated proto/*.pb.go
# as-is - this build never invokes protoc/`make proto` itself, so protoc
# isn't needed in this image; regenerate locally first (see README, "Как
# перегенерировать proto") if proto/pricereader.proto changed.
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# main.go sits directly in cmd/ (same layout as gateway and users).
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/price_reader ./cmd

# --- runtime stage ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/price_reader ./price_reader

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/price_reader", "--healthcheck"]

USER nobody
EXPOSE 8081
ENTRYPOINT ["./price_reader"]
