# Build Stage - VictoriaMetrics wrapper
FROM golang:1.22 AS victoria-metrics

WORKDIR /app
ENV GO111MODULE=on
ENV GOARCH=amd64
ENV GOOS=linux

# Copier tout le contenu du répertoire metrics
COPY metrics/ .

RUN go mod tidy
RUN GOARCH=amd64 GOOS=linux go mod download
RUN GOARCH=amd64 GOOS=linux go build -o wrapper .
RUN chmod +x ./wrapper

# -------------------------------------------------------------------------------------------------
# Steam Stage Handling
ARG TARGETARCH

FROM --platform=$BUILDPLATFORM cm2network/steamcmd:latest AS steam-amd64
FROM --platform=$BUILDPLATFORM scratch AS steam-arm64
COPY docker/steamclient.not-supported /home/steam/steamcmd/linux64/

FROM steam-${TARGETARCH} AS steam

# -------------------------------------------------------------------------------------------------
# Build Stage - Build AssettoServer
FROM --platform=$BUILDPLATFORM mcr.microsoft.com/dotnet/sdk:9.0-alpine AS build
ARG TARGETARCH

RUN apk add --update --no-cache bash

SHELL ["/bin/bash", "-c"]

RUN echo linux-${TARGETARCH/amd/x} > ~/.RuntimeIdentifier

WORKDIR /app

COPY . ./

RUN dotnet publish -r $(cat ~/.RuntimeIdentifier) -c Release --no-self-contained

# -------------------------------------------------------------------------------------------------
# Prepare Stage - Runtime image
FROM mcr.microsoft.com/dotnet/aspnet:9.0

# Create non-root user
RUN groupadd -r acserver && useradd -r -g acserver -u 1000 acserver

WORKDIR /app

ARG STEAM_USERNAME
ARG STEAM_PASSWORD

# Create steam directory
RUN mkdir -p /home/acserver/.steam/sdk64

# Copy steam client from steam stage
COPY --from=steam /home/steam/steamcmd/linux64/steamclient.* /home/acserver/.steam/sdk64/

# Copy VictoriaMetrics wrapper
COPY --from=victoria-metrics /app/wrapper .

# Copy AssettoServer and set permissions
COPY --from=build /app/out-linux-* ./AssettoServer/

# Copy startup scripts
COPY scripts/start-server.sh .

# Copy steam script
COPY scripts/steam-script.txt .
RUN sed -i "s/\${STEAM_USERNAME}/${STEAM_USERNAME}/g" steam-script.txt && \
    sed -i "s/\${STEAM_PASSWORD}/${STEAM_PASSWORD}/g" steam-script.txt

# Create shared-config directory with proper permissions
RUN mkdir -p /shared-config && \
    mkdir -p /var/log/acserver && \
    chmod +x /app/start-server.sh && \
    chmod +x /app/wrapper

# Add steamclient.so fallback location
RUN mkdir -p /app/AssettoServer/ && \
    cp /home/acserver/.steam/sdk64/steamclient.so /app/AssettoServer/ && \
    chmod +x /app/AssettoServer/steamclient.so && \
    chmod +x /app/AssettoServer/AssettoServer

# Set proper permissions for all directories and files
RUN chown -R acserver:acserver /home/acserver && \
    chown -R acserver:acserver /app && \
    chown -R acserver:acserver /shared-config && \
    chown -R acserver:acserver /var/log/acserver && \
    chmod -R 755 /app /shared-config && \
    chmod -R 755 /var/log/acserver

# Create log volume
VOLUME ["/var/log/acserver"]

# Environment variables
ENV GAMESERVER_ID=Unknown
ENV GAMESERVER_REGION=Unknown
ENV SERVER_NAME=Unknown
ENV SERVER_TYPE=Private

# Authentication configuration
ENV AUTH_STEAM_ID=""
ENV AUTH_USER_ID=""

# Victoria Metrics configuration
ENV VICTORIA_METRICS_URL=localhost
ENV VICTORIA_METRICS_PORT=8428
ENV VICTORIA_METRICS_USERNAME=username
ENV VICTORIA_METRICS_PASSWORD=password
ENV VICTORIA_METRICS_REQUEST_TIMEOUT=10s
ENV VICTORIA_METRICS_CONNECT_TIMEOUT=5s
ENV VICTORIA_METRICS_MAX_RETRIES=3
ENV VICTORIA_METRICS_RETRY_BACKOFF=1s

# Victoria Logs configuration
ENV VICTORIA_LOGS_URL=localhost
ENV VICTORIA_LOGS_PORT=9428
ENV VICTORIA_LOGS_USERNAME=username
ENV VICTORIA_LOGS_PASSWORD=password

# Metrics configuration
ENV METRICS_BATCH_SIZE=1000
ENV METRICS_FLUSH_INTERVAL=15s
ENV METRICS_BUFFER_SIZE=10000
ENV METRICS_RETENTION_TIME=24h
ENV METRICS_COMPRESSION=true

# Victoria connection configuration
ENV VICTORIA_REQUEST_TIMEOUT=10s
ENV VICTORIA_CONNECT_TIMEOUT=5s
ENV VICTORIA_MAX_RETRIES=3
ENV VICTORIA_RETRY_BACKOFF=1s
ENV VICTORIA_COMPRESSION=true

# Logging configuration
ENV LOG_DIRECTORY=/var/log/acserver
ENV LOG_MAX_FILE_SIZE=100MB
ENV LOG_MAX_FILES=10
ENV LOG_PATTERNS="error.log,server.log,access.log,chat.log,admin.log,connection.log"

# Add the test script
COPY scripts/test-script.sh .
RUN chmod +x /app/test-script.sh

# Switch to non-root user
USER acserver

# Set the working directory
WORKDIR /app

# Use the wrapper as the entrypoint
ENTRYPOINT ["/app/wrapper", "-i", "/app/start-server.sh"]

