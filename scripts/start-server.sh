#!/bin/bash

# Enable verbose mode for debugging
set -x

# Check if steamclient.so exists
if [ ! -f /home/acserver/.steam/sdk64/steamclient.so ]; then
    echo "Error: steamclient.so not found!"
    exit 1
fi

# Ensure the AssettoServer directory exists and has correct permissions
cd /app/AssettoServer || exit 1

# Copy entire config structure from /shared-config to current directory
echo "Copying config from /shared-config to /app/AssettoServer..."
cp -rfv /shared-config/* . || echo "Warning: Could not copy config files"

echo "Setting proper permissions for all files..."
find . -type f -exec chmod 644 {} \;
find . -type d -exec chmod 755 {} \;

# Make sure the server executable is executable
chmod +x ./AssettoServer

# Start Assetto Corsa Server
echo "Starting Assetto Corsa Server..."
./AssettoServer --plugins-from-workdir