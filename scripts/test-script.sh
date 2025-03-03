#!/bin/bash

echo "Test script started"
echo "Current directory: $(pwd)"
echo "User: $(whoami)"
echo "Date: $(date)"
echo "Environment variables:"
env | sort
echo "Test script completed successfully"

# Sleep to keep the script running
sleep 300 