#!/bin/bash

cd /app/AssettoServer

# Copy config from /shared-config to /app/AssettoServer/cfg
echo "Copying config from /shared-config to /app/AssettoServer/cfg..."
cp /shared-config/* .

# Start Assetto Corsa Server
echo "Starting Assetto Corsa Server..."
./AssettoServer --plugins-from-workdir