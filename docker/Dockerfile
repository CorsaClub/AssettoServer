# Build Stage - Agones SDK wrapper
FROM golang:1.20.4 AS agones-sdk

WORKDIR /app
ENV GO111MODULE=on

COPY agones/go.mod .
COPY agones/go.sum .
COPY agones/main.go .

RUN go mod download
RUN go build -o wrapper .
RUN chmod +x ./wrapper

# -------------------------------------------------------------------------------------------------
# Steam Stage [arm64] - Dummy since Steam doesn't support arm64
FROM --platform=$BUILDPLATFORM scratch AS steam-arm64
COPY docker/steamclient.not-supported /home/steam/steamcmd/linux64/

# -------------------------------------------------------------------------------------------------
# Steam Stage [amd64] - Download Steam client
FROM --platform=$BUILDPLATFORM cm2network/steamcmd:latest AS steam-amd64

FROM --platform=$BUILDPLATFORM steam-${TARGETARCH} AS steam
ARG TARGETARCH

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

# Copy Steam client
COPY --from=steam /home/steam/steamcmd/linux64/steamclient.* /root/.steam/sdk64/

# Copy Agones SDK wrapper
COPY --from=agones-sdk /app/wrapper .

# Copy AssettoServer and set permissions
COPY --from=build /app/out-linux-* ./AssettoServer/
RUN chown -R acserver:acserver /app && \
    chmod +x /app/AssettoServer/AssettoServer

# Copy startup scripts
COPY scripts/start-server.sh .
RUN chmod +x start-server.sh

# Copy steam script
COPY scripts/steam-script.txt .
RUN sed -i "s/\${STEAM_USERNAME}/${STEAM_USERNAME}/g" steam-script.txt && \
    sed -i "s/\${STEAM_PASSWORD}/${STEAM_PASSWORD}/g" steam-script.txt

WORKDIR /data

# Switch to non-root user
USER acserver

ENTRYPOINT ["/app/wrapper", "-i", "/app/start-server.sh"]

