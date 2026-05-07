# syntax=docker/dockerfile:1.6

FROM golang:1.21-alpine AS builder

ARG REVISION=docker
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w -X main.revision=${REVISION}" \
    -o /out/finance-tracker-bot ./app

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S -G app app && \
    mkdir -p /data && chown app:app /data

ENV DATA_FILE_PATH=/data/data.db

COPY --from=builder /out/finance-tracker-bot /usr/local/bin/finance-tracker-bot
COPY schema.sql /usr/local/share/finance-tracker-bot/schema.sql

USER app
WORKDIR /data
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/finance-tracker-bot"]
