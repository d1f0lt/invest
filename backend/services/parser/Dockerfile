# --- build stage ---
FROM golang:1.24-alpine AS build
WORKDIR /src

# Build context is this service's own directory again (docker-compose.yml
# points context/dockerfile back here) - the generated gRPC/protobuf code
# (portfolio's contract, since parser is a portfolio gRPC client) used to
# live in a sibling module (backend/proto), which forced the context out
# to the repo root; it's now committed, generated code in
# internal/portfoliopb - NOT a static copy of the .proto itself (see
# architecture-decisions.md, "перевод внутреннего взаимодействия
# сервисов на gRPC", раздел про пересмотр 2026-09-22). `make proto` in
# this directory is what (re)populates that folder from the canonical
# .proto in ../portfolio/proto/ - it copies the source .proto in, runs
# protoc, then deletes the copy, so on disk here it's only ever
# *.pb.go/*_grpc.pb.go. `COPY . .` below just picks up whatever's
# already generated and committed; this build never invokes
# protoc/`make proto`/`make clean` itself, so protoc isn't needed in
# this image - but that also means internal/portfoliopb must NOT be left
# in the `make clean`-ed (empty) state when building, or this COPY
# brings in nothing to build against.
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/parser ./cmd

# --- runtime stage ---
FROM alpine:3.20
# No wget here anymore: this service's health check is gRPC-only now
# (internal/health), and its Docker HEALTHCHECK below calls its own
# binary's --healthcheck flag instead. See architecture-decisions.md,
# "перевод внутреннего взаимодействия сервисов на gRPC".
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/parser ./parser

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/parser", "--healthcheck"]

USER nobody
ENTRYPOINT ["./parser"]
