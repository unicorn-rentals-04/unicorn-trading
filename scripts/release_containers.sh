#!/bin/bash

set -e
function log() {
    echo "---> ${1}"
}

VERSION=$(cat VERSION)

# Authenticate to dockerhub
echo $PAT_GITHUB | docker login ghcr.io -u $DOCKERHUB_USERNAME --password-stdin


readonly repo_base="ghcr.io/unicorn-rentals-04/unicorn-trading"


## Build Docker Images
docker build -t "${repo_base}/reporter-be:latest" -f docker/backend .
docker build -t "${repo_base}/reporter-fe:latest" -f docker/frontend .

## Tag to version
docker tag "${repo_base}/reporter-be:latest" "${repo_base}/reporter-be:v${VERSION}"
docker tag "${repo_base}/reporter-fe:latest" "${repo_base}/reporter-fe:v${VERSION}"

## Push Images
docker push "${repo_base}/reporter-be:latest"
docker push "${repo_base}/reporter-fe:latest"
docker push "${repo_base}/reporter-be:v${VERSION}"
docker push "${repo_base}/reporter-fe:v${VERSION}"
