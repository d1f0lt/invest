# --- build stage ---
FROM golang:1.24-alpine AS build
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/price_updater ./cmd

# --- runtime stage ---
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/price_updater ./price_updater

USER nobody
ENTRYPOINT ["./price_updater"]
