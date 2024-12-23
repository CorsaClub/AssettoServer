#!/bin/bash

# Usage: ./cleanup-versions.sh <package_type> <package_name> <env> [keep_versions]
# Example: ./cleanup-versions.sh "helm" "server-manager" "prod" 2

set -e

PACKAGE_TYPE=$1  # "helm" ou "docker"
PACKAGE_NAME=$2
ENV=${3:-staging}
KEEP_VERSIONS=${4:-2}

REPO_OWNER=$(echo "$GITHUB_REPOSITORY" | cut -d '/' -f 1 | tr '[:upper:]' '[:lower:]')
REPO_NAME=$(echo "${{ github.repository }}" | cut -d '/' -f 2 | tr '[:upper:]' '[:lower:]')

echo "Cleaning up old versions for ${PACKAGE_NAME}..."

if [ "$PACKAGE_TYPE" = "helm" ]; then
    API_PATH="helm-charts%2F${REPO_NAME}"
    PACKAGE_PREFIX="${PACKAGE_NAME}.${ENV}-"
else
    API_PATH="${PACKAGE_NAME}${ENV}"
    PACKAGE_PREFIX=""
fi

# Lister toutes les versions
VERSIONS=$(curl -s -H "Authorization: Bearer $GITHUB_TOKEN" \
    -H "Accept: application/vnd.github+json" \
    "https://api.github.com/user/packages/container/${API_PATH}/versions" | \
    jq -r '.[].metadata.container.tags[]' | grep -v "latest" | sort -V)

VERSION_COUNT=$(echo "$VERSIONS" | wc -l)

if [ "$VERSION_COUNT" -gt "$KEEP_VERSIONS" ]; then
    VERSIONS_TO_DELETE=$(echo "$VERSIONS" | head -n $(($VERSION_COUNT - $KEEP_VERSIONS)))
    
    echo "Versions to delete:"
    echo "$VERSIONS_TO_DELETE"
    
    echo "$VERSIONS_TO_DELETE" | while read VERSION; do
        if [ ! -z "$VERSION" ]; then
            echo "Deleting version: $VERSION"
            VERSION_ID=$(curl -s -H "Authorization: Bearer $GITHUB_TOKEN" \
                -H "Accept: application/vnd.github+json" \
                "https://api.github.com/user/packages/container/${API_PATH}/versions" | \
                jq -r ".[] | select(.metadata.container.tags[] == \"${VERSION}\") | .id")
            
            if [ ! -z "$VERSION_ID" ]; then
                curl -X DELETE -s \
                    -H "Authorization: Bearer $GITHUB_TOKEN" \
                    -H "Accept: application/vnd.github+json" \
                    "https://api.github.com/user/packages/container/${API_PATH}/versions/${VERSION_ID}"
            fi
        fi
    done
fi 