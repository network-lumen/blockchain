FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates curl jq bash tzdata && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

ARG LUMEND_SRC=./dist/lumend
COPY ${LUMEND_SRC} /usr/local/bin/lumend
RUN chmod +x /usr/local/bin/lumend

ENV LUMEN_HOME=/root/.lumen
EXPOSE 26656 26657 1317 9090
ENTRYPOINT ["/usr/local/bin/lumend"]
