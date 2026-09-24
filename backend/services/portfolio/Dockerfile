FROM golang:1.24-alpine3.22 AS build

# *.pb.go не хранятся в git - генерируются здесь из proto/ (make proto).
RUN apk add --no-cache make protoc protobuf-dev
ARG PROTOC_GEN_GO_VERSION=v1.36.5
ARG PROTOC_GEN_GO_GRPC_VERSION=v1.5.1
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION} \
 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make clean proto

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/portfolio ./cmd

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/portfolio ./portfolio

HEALTHCHECK --interval=10s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/app/portfolio", "--healthcheck"]

USER nobody
ENTRYPOINT ["./portfolio"]
