FROM golang:1.24-alpine3.22 AS build

# *.pb.go не хранятся в git - генерируются здесь (make proto) из .proto
# сервисов-владельцев. Они приходят именованными build-контекстами
# users_proto / portfolio_proto / pricereader_proto (docker-compose.yml:
# build.additional_contexts), т.к. лежат вне контекста gateway. Без compose:
#   docker build --build-context users_proto=../users/proto \
#     --build-context portfolio_proto=../portfolio/proto \
#     --build-context pricereader_proto=../price_reader/proto .
RUN apk add --no-cache make protoc protobuf-dev
ARG PROTOC_GEN_GO_VERSION=v1.36.5
ARG PROTOC_GEN_GO_GRPC_VERSION=v1.5.1
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION} \
 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=users_proto users.proto /proto/users.proto
COPY --from=portfolio_proto portfolio.proto /proto/portfolio.proto
COPY --from=pricereader_proto pricereader.proto /proto/pricereader.proto
RUN make clean proto \
    USERS_SRC=/proto/users.proto \
    PORTFOLIO_SRC=/proto/portfolio.proto \
    PRICEREADER_SRC=/proto/pricereader.proto

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./cmd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget
WORKDIR /app
COPY --from=build /out/gateway ./gateway

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -q -O- http://localhost:8080/healthz || exit 1

USER nobody
ENTRYPOINT ["./gateway"]
