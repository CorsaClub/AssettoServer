#!/bin/bash

# Usage: ./check-modified-files.sh <path_pattern1> [path_pattern2 ...]
# Example: ./check-modified-files.sh "assetto-corsa/helm/server_manager/" ".github/workflows/ac-helm-server_manager.yml"
# Returns: Sets should_run=true in GITHUB_OUTPUT if any of the patterns match

set -e

# Vérifier qu'au moins un pattern est fourni
if [ $# -eq 0 ]; then
    echo "Error: At least one path pattern must be provided"
    echo "Usage: $0 <path_pattern1> [path_pattern2 ...]"
    exit 1
fi

# Générer la liste des fichiers modifiés
if [[ $GITHUB_EVENT_NAME == 'push' ]]; then
    # Pour un push normal
    git diff --name-only $GITHUB_EVENT_BEFORE $GITHUB_EVENT_AFTER > changed_files.txt
else
    # Pour un tag ou autre
    git diff --name-only HEAD^ HEAD > changed_files.txt
fi

echo "Changed files:"
cat changed_files.txt

# Vérifier chaque pattern fourni
should_run=false
for pattern in "$@"; do
    if grep -q "$pattern" changed_files.txt; then
        should_run=true
        echo "Match found for pattern: $pattern"
        break
    fi
done

echo "should_run=$should_run" >> $GITHUB_OUTPUT 