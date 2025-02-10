#!/bin/bash

# Enable verbose mode for debugging
set -x

# Check if steamclient.so exists
if [ ! -f /home/acserver/.steam/sdk64/steamclient.so ]; then
    echo "steamclient.so not found in .steam/sdk64, checking fallback location..."
    if [ -f /app/AssettoServer/steamclient.so ]; then
        echo "Using fallback steamclient.so"
        mkdir -p /home/acserver/.steam/sdk64
        cp /app/AssettoServer/steamclient.so /home/acserver/.steam/sdk64/
    else
        echo "Error: steamclient.so not found in any location!"
        exit 1
    fi
fi

# Ensure proper permissions for Steam files
chmod 755 /home/acserver/.steam/sdk64/steamclient.so || echo "Warning: Could not set steamclient.so permissions"

# Ensure the AssettoServer directory exists and has correct permissions
cd /app/AssettoServer || exit 1

# Copy entire config structure from /shared-config to current directory
echo "Copying config from /shared-config to /app/AssettoServer..."
cp -rfv /shared-config/* . || echo "Warning: Could not copy config files"

echo "Setting proper permissions for all files..."
find . -type f -not -name "steamclient.so" -exec chmod 644 {} \;
find . -type d -exec chmod 755 {} \;

# Make sure the server executable is executable
chmod +x ./AssettoServer

# Start Assetto Corsa Server
echo "Starting Assetto Corsa Server..."
./AssettoServer --plugins-from-workdir