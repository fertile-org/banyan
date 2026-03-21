#!/bin/bash
set -e

echo "========================================="
echo "Banyan Engine HA Container (${ENGINE_ID})"
echo "========================================="

ENGINE_ID=${ENGINE_ID:-engine-1}
ENGINE_GRPC_PORT=${ENGINE_GRPC_PORT:-50051}
ETCD_ENDPOINT=${ETCD_ENDPOINT:-http://172.28.0.5:2379}

# 1. Write multi-engine config (external etcd, no managed etcd/registry)
echo "Writing config..."
mkdir -p /etc/banyan /etc/banyan/whitelisted-keys /var/lib/banyan
cat > /etc/banyan/banyan.yaml <<EOF
engine:
    grpc_port: "${ENGINE_GRPC_PORT}"
    store_backend: "etcd"
    store_address: "${ETCD_ENDPOINT}"
    managed_etcd: false
    managed_registry: false
    external_registry_url: "172.28.0.10:5000"
    multi_engine: true
    engine_id: "${ENGINE_ID}"
cli:
    engine_host: "127.0.0.1"
    engine_port: "${ENGINE_GRPC_PORT}"
    engines:
    - address: "127.0.0.1:${ENGINE_GRPC_PORT}"
      wg_public_key: "dummy-for-e2e"
EOF
chmod 600 /etc/banyan/banyan.yaml

# 2. Signal that this engine's config is ready
echo "${ENGINE_ID}" > /tmp/keys-exchange/${ENGINE_ID}-ready-config
echo "Config written for ${ENGINE_ID}"

# 3. Wait for both engines to have written configs
echo "Waiting for all engine configs..."
while [ ! -f /tmp/keys-exchange/engine-1-ready-config ] || [ ! -f /tmp/keys-exchange/engine-2-ready-config ]; do
    sleep 1
done

# 4. Wait for workers to signal ready
echo "Waiting for worker ready signals..."
while [ ! -f /tmp/keys-exchange/worker-1-ready ] || [ ! -f /tmp/keys-exchange/worker-2-ready ]; do
    sleep 1
done

echo "Starting engine ${ENGINE_ID}..."

# 5. Start engine in insecure mode (no WireGuard for multi-engine E2E)
banyan-engine start --allow-insecure &
ENGINE_PID=$!

# Forward signals
trap "kill -TERM $ENGINE_PID 2>/dev/null; wait $ENGINE_PID 2>/dev/null; exit" SIGTERM SIGINT

# 6. Wait for gRPC to be ready, then signal
echo "Waiting for ${ENGINE_ID} gRPC to be ready..."
until nc -z 0.0.0.0 ${ENGINE_GRPC_PORT} 2>/dev/null; do
    sleep 1
done
touch /tmp/keys-exchange/${ENGINE_ID}-grpc-ready
echo "${ENGINE_ID} is ready."

# Wait for engine process
wait $ENGINE_PID
