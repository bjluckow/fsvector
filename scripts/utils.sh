#!/usr/bin/env bash

fsvectord() {
    set -a && source .env && set +a
    go build -o ./bin/fsvectord ./cmd/fsvectord && ./bin/fsvectord "$@"
}