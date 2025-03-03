#!/bin/bash

# Disable verbose mode temporarily to make logs cleaner
set +x

echo "==============================================="
echo "Starting Assetto Corsa Server Wrapper"
echo "==============================================="
echo "Current directory: $(pwd)"
echo "User: $(whoami)"
echo "Date: $(date)"

# Re-enable verbose mode
set -x

# Wait for initialization to complete
while [ ! -f /shared-config/init_done ]; do
    echo "Waiting for initialization to complete..."
    sleep 2
done

echo "Initialization complete, proceeding with server startup"

# Check for steamclient.so
if [ ! -f /home/acserver/.steam/sdk64/steamclient.so ]; then
    echo "Setting up Steam client library..."
    mkdir -p /home/acserver/.steam/sdk64
    if [ -f /app/AssettoServer/steamclient.so ]; then
        cp /app/AssettoServer/steamclient.so /home/acserver/.steam/sdk64/
        chmod 755 /home/acserver/.steam/sdk64/steamclient.so
    else
        echo "ERROR: steamclient.so not found!"
        exit 1
    fi
fi

# Change to server directory
echo "Changing to server directory..."
if [ ! -d /app/AssettoServer ]; then
    echo "ERROR: Server directory not found!"
    ls -la /app
    exit 1
fi

cd /app/AssettoServer || exit 1
echo "Current directory: $(pwd)"

# Copy configuration
echo "Copying configuration files..."
cp -rf /shared-config/* . || echo "Warning: Some files could not be copied"

# Set permissions
echo "Setting file permissions..."
find . -type f -not -path "*/\.*" -exec chmod 644 {} \;
find . -type d -not -path "*/\.*" -exec chmod 755 {} \;
chmod +x ./AssettoServer

# Verify server executable
if [ ! -f ./AssettoServer ]; then
    echo "ERROR: AssettoServer executable not found!"
    ls -la
    exit 1
fi

echo "Server executable found: $(ls -la ./AssettoServer)"

# Check for critical configuration files
if [ ! -f ./cfg/server_cfg.ini ]; then
    echo "ERROR: server_cfg.ini not found!"
    ls -la ./cfg/
    exit 1
fi

echo "==============================================="
echo "Starting Assetto Corsa Server"
echo "==============================================="

# Execute the server with exec to replace the current process
exec ./AssettoServer --plugins-from-workdir