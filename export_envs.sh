#!/bin/bash

# Check if .env file exists
if [ ! -f .env ]; then
  echo ".env file not found!"
  exit 1
fi

# Export all valid key-value pairs from .env file
export $(grep -vE '^\s*#' .env | sed '/^\s*$/d' | sed 's/#.*//' | xargs -d '\n')

echo "Environment variables from .env have been loaded into the current shell session."

