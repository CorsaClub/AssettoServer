#!/bin/bash

cd /app/acServer

echo "Creating directories 'content' and 'cfg'..."
mkdir -p content cfg

# Copie le contenu de /shared-mods vers /app/acServer/content
echo "Copying mods from /shared-mods to /app/acServer/content..."
cp -R /shared-mods/* content/

# Copie le contenu de /shared-config vers /app/acServer/cfg
echo "Copying config from /shared-config to /app/acServer/cfg..."
cp /shared-config/* cfg/

# Lance le serveur Assetto Corsa
echo "Starting Assetto Corsa Server..."
./AssettoServer --plugins-from-workdir