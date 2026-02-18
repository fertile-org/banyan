#!/bin/bash
set -e

echo "========================================="
echo "Banyan Agent Container"
echo "========================================="

# Get node name from environment or hostname
NODE_NAME=${NODE_NAME:-$(hostname)}
ENGINE_ENDPOINT=${ENGINE_ENDPOINT:-http://engine:2379}

# Wait for engine to be ready
echo "Waiting for engine at $ENGINE_ENDPOINT..."
until curl -sf "$ENGINE_ENDPOINT/health" > /dev/null 2>&1 || etcdctl --endpoints="$ENGINE_ENDPOINT" endpoint health > /dev/null 2>&1; do
    echo "Engine not ready, waiting..."
    sleep 2
done
echo "Engine is ready!"

# Initialize agent
echo "Initializing agent..."
banyan-cli agent init

# Start containerd in background
echo "Starting containerd..."
containerd &
sleep 2

# Start agent
echo "Starting agent..."
exec banyan-cli agent start --engine "$ENGINE_ENDPOINT" --node-name "$NODE_NAME"
