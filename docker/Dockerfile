# ── build stage ───────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o /out/fsvectord ./cmd/fsvectord
RUN go build -o /out/fsvector  ./cmd/fsvector

# ── fsvectord runtime ─────────────────────────────────────────────────────────
FROM alpine:3.19 AS fsvectord

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /out/fsvectord /usr/local/bin/fsvectord

ENTRYPOINT ["fsvectord"]

