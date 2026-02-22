#!/bin/bash
set -e

echo "========================================="
echo "Banyan Engine Container"
echo "========================================="

# Default E2E password (override with E2E_PASSWORD env var)
E2E_PASSWORD=${E2E_PASSWORD:-banyan-e2e-secret}

# Write config file (everything except password — that goes via --password flag)
echo "Writing config..."
mkdir -p /etc/banyan
cat > /etc/banyan/banyan.yaml <<EOF
engine:
    grpc_port: "50051"
    store_backend: "etcd"
    managed_etcd: true
cli:
    engine_host: 127.0.0.1
    engine_port: "50051"
EOF
chmod 600 /etc/banyan/banyan.yaml

# Initialize engine (password via flag to avoid interactive prompt)
echo "Initializing engine..."
banyan-engine init --password "${E2E_PASSWORD}"

# Start engine in background so we can authenticate the CLI
echo "Starting engine..."
banyan-engine start &
ENGINE_PID=$!

# Forward signals to the engine process
trap "kill -TERM $ENGINE_PID 2>/dev/null; wait $ENGINE_PID 2>/dev/null; exit" SIGTERM SIGINT

# Wait for engine gRPC to be ready
echo "Waiting for engine gRPC to be ready..."
until nc -z 127.0.0.1 50051 2>/dev/null; do
    sleep 1
done

# Authenticate the CLI (exchange password for token)
echo "Authenticating CLI..."
banyan-cli auth --password "${E2E_PASSWORD}"

echo "Engine is ready."

# Wait for the engine process (keeps the container running)
wait $ENGINE_PID
