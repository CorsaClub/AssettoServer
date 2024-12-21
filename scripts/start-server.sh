#!/bin/bash

cd /app/AssettoServer

# Copie le contenu de /shared-config vers /app/acServer/cfg
echo "Copying config from /shared-config to /app/acServer/cfg..."
cp /shared-config/* .

# Lance le serveur Assetto Corsa
echo "Starting Assetto Corsa Server..."
./AssettoServer --plugins-from-workdir