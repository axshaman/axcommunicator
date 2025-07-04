FROM golang:1.24.0 as dev

RUN apt-get update && apt-get install -y \
    git \
    curl \
    && rm -rf /var/lib/apt/lists/*

RUN groupadd -r appgroup && \
    useradd -r -g appgroup -d /app appuser -u 1000 && \
    mkdir -p \
    /app/database \
    /app/storage/temp \
    /app/logs \
    /app/migrations \
    /app/tmp \
    /app/tmp/.cache \
    /.cache/go-build && \
    chown -R 1000:appgroup /app && \
    chown -R 1000:appgroup /.cache && \
    chmod -R 775 /app/tmp && \
    chmod -R 775 /.cache

WORKDIR /app

COPY --chown=1000:appgroup go.mod go.sum ./
RUN go mod download

COPY --chown=1000:appgroup . .

HEALTHCHECK --interval=30s --timeout=3s \
    CMD curl -f http://localhost:8600/api/v1/health || exit 1
EXPOSE 8600

USER 1000
ENV GOCACHE=/app/tmp/.cache \
    GOMODCACHE=/go/pkg/mod

RUN go build -o /axcommutator
CMD ["/axcommutator"]
