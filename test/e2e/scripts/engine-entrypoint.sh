#!/bin/bash
set -e

echo "========================================="
echo "Banyan Engine Container"
echo "========================================="

# 1. Write base config
echo "Writing config..."
mkdir -p /etc/banyan /etc/banyan/whitelisted-keys
cat > /etc/banyan/banyan.yaml <<EOF
engine:
    grpc_port: "50051"
    store_backend: "etcd"
    managed_etcd: true
    overlay_type: "vxlan"
EOF
chmod 600 /etc/banyan/banyan.yaml

# 2. Init engine (generates WireGuard keypair, no --password)
echo "Initializing engine..."
banyan-engine init

# 3. Export engine public key so workers and CLI can read it
ENGINE_PUB_KEY=$(grep 'wg_public_key' /etc/banyan/banyan.yaml | head -1 | awk '{print $2}')
echo "$ENGINE_PUB_KEY" > /tmp/keys-exchange/engine.pub
echo "Engine public key: $ENGINE_PUB_KEY"

# 4. Generate CLI keypair (engine and CLI share the same container in E2E)
echo "Generating CLI keypair..."
CLI_PRIV_KEY=$(wg genkey)
CLI_PUB_KEY=$(echo "$CLI_PRIV_KEY" | wg pubkey)
echo "CLI public key: $CLI_PUB_KEY"

# Write CLI config section into the existing config
# We need to merge it with the engine config, so use a temp approach
python3 -c "
import yaml, sys
with open('/etc/banyan/banyan.yaml') as f:
    cfg = yaml.safe_load(f)
cfg['cli'] = {
    'engine_host': '127.0.0.1',
    'engine_port': '50051',
    'name': 'cli-engine',
    'wg_private_key': '$CLI_PRIV_KEY',
    'wg_public_key': '$CLI_PUB_KEY',
}
with open('/etc/banyan/banyan.yaml', 'w') as f:
    yaml.dump(cfg, f, default_flow_style=False)
" 2>/dev/null || {
    # Fallback: append CLI section manually if python3/yaml not available
    cat >> /etc/banyan/banyan.yaml <<EOF2
cli:
    engine_host: 127.0.0.1
    engine_port: "50051"
    name: cli-engine
    wg_private_key: $CLI_PRIV_KEY
    wg_public_key: $CLI_PUB_KEY
EOF2
}

# Whitelist CLI key immediately
echo "$CLI_PUB_KEY" > /etc/banyan/whitelisted-keys/cli-engine.pub

# 5. Wait for workers to write their keys
echo "Waiting for worker keys..."
while [ ! -f /tmp/keys-exchange/worker-1.pub ] || [ ! -f /tmp/keys-exchange/worker-2.pub ]; do
    sleep 1
done

# 6. Whitelist worker keys
cp /tmp/keys-exchange/worker-1.pub /etc/banyan/whitelisted-keys/worker-1.pub
cp /tmp/keys-exchange/worker-2.pub /etc/banyan/whitelisted-keys/worker-2.pub
echo "All keys whitelisted."

# 7. Start engine
echo "Starting engine..."
banyan-engine start &
ENGINE_PID=$!

# Forward signals to the engine process
trap "kill -TERM $ENGINE_PID 2>/dev/null; wait $ENGINE_PID 2>/dev/null; exit" SIGTERM SIGINT

# 8. Wait for gRPC to be ready, then signal workers
echo "Waiting for engine gRPC to be ready..."
until nc -z 127.0.0.1 50051 2>/dev/null; do
    sleep 1
done
touch /tmp/keys-exchange/engine-ready
echo "Engine is ready."

# Wait for the engine process (keeps the container running)
wait $ENGINE_PID
