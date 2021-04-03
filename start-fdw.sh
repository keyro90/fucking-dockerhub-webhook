#!/usr/bin/env bash

while true; do
  ./fucking-dockerhub-webhook
  sleep 1
  echo "Fucking dockerhub webhook failed. Retry."
done