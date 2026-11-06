FROM golang:1.24-alpine3.22 AS build

RUN apk add --no-cache make protoc protobuf-dev
ARG PROTOC_GEN_GO_VERSION=v1.36.5
ARG PROTOC_GEN_GO_GRPC_VERSION=v1.5.1
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION} \
 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=portfolio_proto portfolio.proto /proto/portfolio.proto
COPY --from=feed_proto feed.proto /proto/feed.proto
RUN make clean proto PORTFOLIO_SRC=/proto/portfolio.proto FEED_SRC=/proto/feed.proto
RUN CGO_ENABLED=0 go build -o /out/assistant ./cmd

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/assistant ./assistant
HEALTHCHECK --interval=10s --timeout=3s CMD wget -q -O- http://127.0.0.1:8087/healthz || exit 1
USER nobody
ENTRYPOINT ["./assistant"]
