# syntax=docker/dockerfile:1.7
ARG GO_VERSION=1.24
FROM golang:${GO_VERSION}-bookworm AS base
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential git curl jq unzip ca-certificates make && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /src

# ---- Dev shell: iterate with mounted source ----
FROM base AS dev
CMD ["/bin/bash"]

# ---- Builder: produce the lumend binaries ----
FROM base AS builder
# Build context must include chain_source/ so go.mod is available
COPY . /src
WORKDIR /src
RUN make build || (echo "Fallback go build"; mkdir -p build && go build -o build/lumend ./cmd/lumend)
# Cross-compile Windows executable alongside Linux binary
RUN CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o build/lumend.exe ./cmd/lumend
RUN install -Dm755 build/lumend /out/lumend && \
    install -Dm755 build/lumend.exe /out/lumend.exe
