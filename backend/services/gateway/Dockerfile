# --- build stage ---
FROM golang:1.24-alpine AS build
WORKDIR /src

# Build context is this service's own directory again (docker-compose.yml
# points context/dockerfile back here) - the generated gRPC/protobuf code
# (users/portfolio/price_reader's contracts, since gateway is a client of
# all three) used to live in a sibling module (backend/proto), which
# forced the context out to the repo root; it's now committed, generated
# code in internal/{userspb,portfoliopb,pricereaderpb} - NOT a static copy
# of the .proto itself (that would still be the old duplication problem;
# see architecture-decisions.md, "перевод внутреннего взаимодействия
# сервисов на gRPC", раздел про пересмотр 2026-09-22). `make proto` in
# this directory is what (re)populates those three folders from the
# canonical .proto in ../{users,portfolio,price_reader}/proto/ - it
# copies the source .proto in, runs protoc, then deletes the copy, so on
# disk here it's only ever *.pb.go/*_grpc.pb.go. `COPY . .` below just
# picks up whatever's already generated and committed; this build never
# invokes protoc/`make proto`/`make clean` itself, so protoc isn't needed
# in this image - but that also means internal/{userspb,portfoliopb,
# pricereaderpb} must NOT be left in the `make clean`-ed (empty) state
# when building, or this COPY brings in nothing to build against.
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# ./cmd, not ./cmd/gateway: this service's main.go lives directly under
# cmd/ (see cmd/main.go's doc comment for why).
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./cmd

# --- runtime stage ---
FROM alpine:3.20
# wget stays here (unlike every other service's Dockerfile): gateway's
# own external API is still HTTP, and its Docker HEALTHCHECK below still
# curls its own GET /healthz - only the internal calls it makes to
# users/portfolio/price_reader became gRPC. See architecture-decisions.md,
# "перевод внутреннего взаимодействия сервисов на gRPC".
RUN apk add --no-cache ca-certificates tzdata wget
WORKDIR /app
COPY --from=build /out/gateway ./gateway

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O- http://localhost:8080/healthz || exit 1

USER nobody
ENTRYPOINT ["./gateway"]
