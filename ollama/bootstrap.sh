#!/bin/sh

set -e

MODEL="qwen2.5:3b"
API="http://ollama:11434"

echo "⏳ Waiting for Ollama..."

# wait for API
until wget -qO- $API/api/tags >/dev/null 2>&1; do
  echo "   waiting for ollama..."
  sleep 2
done

echo "✅ Ollama is ready"

# check model
MODEL_EXISTS=$(wget -qO- $API/api/tags | grep -q "$MODEL" && echo "yes" || echo "no")

if [ "$MODEL_EXISTS" = "yes" ]; then
  echo "✅ Model already present: $MODEL"
  exit 0
fi

echo "📦 Model missing: $MODEL"
echo "⬇️ Starting pull..."

# trigger pull
wget -qO- \
  --header="Content-Type: application/json" \
  --post-data="{\"name\":\"$MODEL\"}" \
  $API/api/pull >/dev/null 2>&1 &

# NOTE: pull runs async, so we poll readiness
while true; do
  STATUS=$(wget -qO- $API/api/tags | grep "$MODEL" || true)

  if [ -n "$STATUS" ]; then
    echo "✅ Model is now available"
    break
  fi

  echo "⏳ downloading model..."
  sleep 5
done

echo "🚀 Bootstrap complete"