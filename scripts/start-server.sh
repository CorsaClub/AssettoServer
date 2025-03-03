#!/bin/bash

# Enable verbose mode for debugging
set -x

# Add more debugging
echo "Starting server script with PID $$"
echo "Current directory: $(pwd)"
echo "User: $(whoami)"
echo "Environment variables:"
env | sort

while [ ! -f /shared-config/init_done ]; do
    echo "Waiting for init_done file..."
    sleep 1
done

echo "Init done file found, server booting..."

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
echo "Changing to AssettoServer directory..."
cd /app/AssettoServer || { echo "Failed to change to /app/AssettoServer directory!"; exit 1; }

# Copy entire config structure from /shared-config to current directory
echo "Copying config from /shared-config to /app/AssettoServer..."
cp -rfv /shared-config/* . || echo "Warning: Could not copy config files"

echo "Setting proper permissions for all files..."
find . -type f -not -name "steamclient.so" -exec chmod 644 {} \;
find . -type d -exec chmod 755 {} \;

# Make sure the server executable is executable
chmod +x ./AssettoServer

# DEBUG
echo "DEBUG: Current directory:"
pwd
echo "DEBUG: Current directory contents:"
ls -la

# Check if the AssettoServer executable exists
if [ ! -f ./AssettoServer ]; then
    echo "ERROR: AssettoServer executable not found!"
    exit 1
fi

echo "DEBUG: AssettoServer file details:"
file ./AssettoServer
ls -la ./AssettoServer

echo "[DEBUG] Check content of server_cfg.ini:"
cat ./cfg/server_cfg.ini || echo "WARNING: server_cfg.ini not found!"

echo "[DEBUG] Check content of entry_list.ini:"
cat ./cfg/entry_list.ini || echo "WARNING: entry_list.ini not found!"

# Start Assetto Corsa Server
echo "Starting Assetto Corsa Server with exec..."
exec ./AssettoServer --plugins-from-workdir