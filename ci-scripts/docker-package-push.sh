#!/bin/bash

# Usage: ./docker-package-push.sh <context_path> <dockerfile_path> <image_name> <env> <version>
# Example: ./docker-package-push.sh "./assetto-corsa/src/server_manager" "./docker/Dockerfile" "server-manager" "staging" "v0.0.55-p23"

set -e

CONTEXT_PATH=$1
DOCKERFILE_PATH=$2
IMAGE_NAME=$3
ENV=${4:-staging}
VERSION=${5:-latest}
shift 5

# Nettoyer les variables d'environnement
ENV=$(echo "$ENV" | tr -d '[:space:]')
VERSION=$(echo "$VERSION" | tr -d '[:space:]')

# Construire le nom complet de l'image avec l'environnement
REPO_OWNER=$(echo "$GITHUB_REPOSITORY" | cut -d '/' -f 1 | tr '[:upper:]' '[:lower:]')
FULL_IMAGE_NAME="${IMAGE_NAME}.${ENV}"
IMAGE_FULL_NAME="ghcr.io/${REPO_OWNER}/${FULL_IMAGE_NAME}:${VERSION}"

# Construire la chaîne des build-args
BUILD_ARGS=""
for arg in "$@"; do
    BUILD_ARGS="$BUILD_ARGS --build-arg $arg"
done

# Build et push de l'image
docker buildx build \
    --platform linux/amd64 \
    --push \
    -t "${IMAGE_FULL_NAME}" \
    -f "${DOCKERFILE_PATH}" \
    ${BUILD_ARGS} \
    "${CONTEXT_PATH}"

if [ "$VERSION" != "latest" ]; then
    docker buildx build \
        --platform linux/amd64 \
        --push \
        -t "ghcr.io/${REPO_OWNER}/${FULL_IMAGE_NAME}:latest" \
        -f "${DOCKERFILE_PATH}" \
        ${BUILD_ARGS} \
        "${CONTEXT_PATH}"
fi

# Output the versions for use in the workflow
{
    echo "image_name=${FULL_IMAGE_NAME}"
    echo "version=${VERSION}"
} >> "$GITHUB_OUTPUT"