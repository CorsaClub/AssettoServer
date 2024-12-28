#!/bin/bash

# Ensure the acServer directory exists and has correct permissions
mkdir -p /app/acServer
chown -R 1000:1000 /app/acServer
chmod -R 755 /app/acServer

cd /app/acServer || exit 1

# Copy entire config structure from /shared-config to current directory
echo "Copying config from /shared-config to /app/acServer..."
cp -rf /shared-config/. . || echo "Warning: Could not copy config files"

echo "Setting proper permissions for all files..."
chown -R 1000:1000 .
find . -type f -exec chmod 644 {} \;
find . -type d -exec chmod 755 {} \;

# Start Assetto Corsa Server
echo "Starting Assetto Corsa Server..."
./AssettoServer --plugins-from-workdir