#!/bin/sh
set -eu
API="http://ollama:11434"
MODELS="qwen2.5:3b nomic-embed-text"
echo "Waiting for Ollama..."
until wget -qO- "$API/api/tags" >/dev/null 2>&1; do sleep 2; done
for MODEL in $MODELS; do
  if wget -qO- "$API/api/tags" | grep -q "$MODEL"; then echo "Model already present: $MODEL"; continue; fi
  echo "Pulling $MODEL (this may take a while)..."
  wget -qO- --header="Content-Type: application/json" --post-data="{\"name\":\"$MODEL\"}" "$API/api/pull" >/dev/null
  until wget -qO- "$API/api/tags" | grep -q "$MODEL"; do sleep 5; done
  echo "Model available: $MODEL"
done
echo "Ollama bootstrap complete"
