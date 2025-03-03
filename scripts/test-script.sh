#!/bin/bash

echo "Test script started"
echo "Current directory: $(pwd)"
echo "User: $(whoami)"
echo "Date: $(date)"
echo "Environment variables:"
env | sort
echo "Test script completed successfully"

# Keep the script running indefinitely
echo "Entering infinite loop to keep container alive"
while true; do
    echo "Test script heartbeat: $(date)"
    sleep 60
done 