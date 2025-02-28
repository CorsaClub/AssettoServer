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

# Set proper permissions
RUN chown -R acserver:acserver /home/acserver

# Copy VictoriaMetrics wrapper
COPY --from=victoria-metrics /app/wrapper .

# Copy AssettoServer and set permissions
COPY --from=build /app/out-linux-* ./AssettoServer/

# Create shared-config directory with proper permissions
RUN mkdir -p /shared-config && \
    mkdir -p /var/log/acserver && \
    chown -R acserver:acserver /app /shared-config && \
    chown -R acserver:acserver /var/log/acserver && \
    chmod -R 755 /app /shared-config && \
    chmod -R 755 /var/log/acserver && \
    chmod +x /app/AssettoServer/AssettoServer

# Copy startup scripts
COPY scripts/start-server.sh .
RUN chmod +x start-server.sh

# Copy steam script
COPY scripts/steam-script.txt .
RUN sed -i "s/\${STEAM_USERNAME}/${STEAM_USERNAME}/g" steam-script.txt && \
    sed -i "s/\${STEAM_PASSWORD}/${STEAM_PASSWORD}/g" steam-script.txt

# Create log volume
VOLUME ["/var/log/acserver"]

WORKDIR /data

# Add steamclient.so fallback location
RUN mkdir -p /app/AssettoServer/ && \
    cp /home/acserver/.steam/sdk64/steamclient.so /app/AssettoServer/ && \
    chmod +x /app/AssettoServer/steamclient.so

# Environment variables
ENV GAMESERVER_ID=Unknown
ENV GAMESERVER_REGION=Unknown
ENV SERVER_NAME=Unknown
ENV SERVER_TYPE=Private

# Victoria Metrics configuration
ENV VICTORIA_URL=localhost
ENV VICTORIA_PORT=8428
ENV VICTORIA_USERNAME=username
ENV VICTORIA_PASSWORD=password

# Metrics configuration
ENV METRICS_BATCH_SIZE=1000
ENV METRICS_FLUSH_INTERVAL=15s
ENV METRICS_BUFFER_SIZE=10000
ENV METRICS_RETENTION_TIME=24h

# Victoria connection configuration
ENV VICTORIA_REQUEST_TIMEOUT=10s
ENV VICTORIA_CONNECT_TIMEOUT=5s
ENV VICTORIA_MAX_RETRIES=3
ENV VICTORIA_RETRY_BACKOFF=1s
ENV VICTORIA_COMPRESSION=true

# Switch to non-root user
USER acserver

ENTRYPOINT ["/app/wrapper", "-i", "/app/start-server.sh"]

