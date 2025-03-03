#!/bin/bash

# Disable verbose mode for cleaner logs
set +x

echo "==============================================="
echo "Starting Assetto Corsa Server Wrapper"
echo "==============================================="
echo "Current directory: $(pwd)"
echo "User: $(whoami)"
echo "Date: $(date)"

# Wait for initialization to complete with better logging
echo "Checking for initialization flag at /shared-config/init_done"
while [ ! -f /shared-config/init_done ]; do
    echo "Waiting for initialization to complete... (init_done flag not found)"
    sleep 5
done

echo "Initialization complete! Found init_done flag."
echo "Proceeding with server startup sequence..."

# Check for steamclient.so with better error handling
echo "Checking Steam client library..."
if [ ! -f /home/acserver/.steam/sdk64/steamclient.so ]; then
    echo "Steam client library not found in primary location."
    mkdir -p /home/acserver/.steam/sdk64
    if [ -f /app/AssettoServer/steamclient.so ]; then
        echo "Found Steam client library in fallback location, copying..."
        cp /app/AssettoServer/steamclient.so /home/acserver/.steam/sdk64/
        chmod 755 /home/acserver/.steam/sdk64/steamclient.so
        echo "Steam client library installed successfully."
    else
        echo "ERROR: Steam client library not found in any location!"
        ls -la /app/AssettoServer/
        ls -la /home/acserver/.steam/
        exit 1
    fi
else
    echo "Steam client library found in primary location."
fi

# Change to server directory with better error handling
echo "Changing to server directory..."
if [ ! -d /app/AssettoServer ]; then
    echo "ERROR: Server directory not found at /app/AssettoServer!"
    echo "Contents of /app:"
    ls -la /app
    exit 1
fi

cd /app/AssettoServer || { echo "Failed to change directory!"; exit 1; }
echo "Successfully changed to server directory: $(pwd)"

# Copy configuration with better feedback
echo "Copying configuration files from /shared-config..."
cp -rf /shared-config/* . || { echo "WARNING: Error copying configuration files!"; ls -la /shared-config; }
echo "Configuration files copied."

# Set permissions with better feedback
echo "Setting file permissions..."
find . -type f -not -path "*/\.*" -exec chmod 644 {} \; || echo "WARNING: Error setting file permissions!"
find . -type d -not -path "*/\.*" -exec chmod 755 {} \; || echo "WARNING: Error setting directory permissions!"
chmod +x ./AssettoServer || { echo "ERROR: Failed to make server executable!"; exit 1; }
echo "File permissions set."

# Verify server executable with better error handling
if [ ! -f ./AssettoServer ]; then
    echo "ERROR: AssettoServer executable not found after configuration!"
    echo "Contents of current directory:"
    ls -la
    exit 1
fi

echo "Server executable verified: $(ls -la ./AssettoServer)"

# Check for critical configuration files with better error handling
if [ ! -f ./cfg/server_cfg.ini ]; then
    echo "ERROR: server_cfg.ini not found!"
    echo "Contents of cfg directory:"
    ls -la ./cfg/ || echo "cfg directory not found!"
    exit 1
fi

echo "Configuration files verified."
echo "==============================================="
echo "Starting Assetto Corsa Server"
echo "==============================================="

# Execute the server with exec to replace the current process
exec ./AssettoServer --plugins-from-workdir