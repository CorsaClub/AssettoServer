#!/bin/bash

# Set proper permissions for the base directory
chmod 755 /app/acServer

cd /app/acServer || exit 1

# Copy entire config structure from /shared-config to current directory
echo "Copying config from /shared-config to /app/acServer..."
cp -rf /shared-config/. . || echo "Warning: Could not copy config files"
chmod -R 644 ./* 2>/dev/null || true
find . -type d -exec chmod 755 {} \; 2>/dev/null || true

# Start Assetto Corsa Server
echo "Starting Assetto Corsa Server..."
./AssettoServer --plugins-from-workdir