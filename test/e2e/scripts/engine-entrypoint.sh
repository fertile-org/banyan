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
    managed_registry: true
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

# Write CLI private key to file
mkdir -p /etc/banyan/keys
echo "$CLI_PRIV_KEY" > /etc/banyan/keys/cli.key
chmod 600 /etc/banyan/keys/cli.key

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
    'wg_private_key_file': '/etc/banyan/keys/cli.key',
    'wg_public_key': '$CLI_PUB_KEY',
    'engine_wg_public_key': '$ENGINE_PUB_KEY',
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
    wg_private_key_file: /etc/banyan/keys/cli.key
    wg_public_key: $CLI_PUB_KEY
    engine_wg_public_key: $ENGINE_PUB_KEY
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
# gRPC binds to the engine's WireGuard tunnel IP (derived from its public key)
ENGINE_HASH=$(echo -n "$ENGINE_PUB_KEY" | sha256sum | head -c 4)
ENGINE_O3=$((16#${ENGINE_HASH:0:2}))
ENGINE_O4=$((16#${ENGINE_HASH:2:2}))
if [ "$ENGINE_O3" -eq 0 ] && [ "$ENGINE_O4" -le 1 ]; then
    ENGINE_O4=$((ENGINE_O4 + 2))
fi
ENGINE_TUNNEL_IP="10.200.${ENGINE_O3}.${ENGINE_O4}"
echo "Engine tunnel IP: $ENGINE_TUNNEL_IP"
echo "Waiting for engine gRPC to be ready..."
until nc -z "$ENGINE_TUNNEL_IP" 50051 2>/dev/null; do
    sleep 1
done
touch /tmp/keys-exchange/engine-ready
echo "Engine is ready."

# 8b. Set up CLI WireGuard control tunnel (banyan-cli commands need this)
echo "Setting up CLI control tunnel..."
# Compute CLI tunnel IP from public key (SHA-256 hash → first 2 bytes → 10.200.x.y)
# This replicates types.TunnelIPFromPublicKey() in Go
CLI_HASH=$(echo -n "$CLI_PUB_KEY" | sha256sum | head -c 4)
CLI_O3=$((16#${CLI_HASH:0:2}))
CLI_O4=$((16#${CLI_HASH:2:2}))
if [ "$CLI_O3" -eq 0 ] && [ "$CLI_O4" -le 1 ]; then
    CLI_O4=$((CLI_O4 + 2))
fi
CLI_TUNNEL_IP="10.200.${CLI_O3}.${CLI_O4}"
echo "CLI tunnel IP: $CLI_TUNNEL_IP"

# Create wg-ctl-cli interface with CLI private key
ip link add wg-ctl-cli type wireguard
wg set wg-ctl-cli private-key /etc/banyan/keys/cli.key
ip addr add "${CLI_TUNNEL_IP}/16" dev wg-ctl-cli
ip link set wg-ctl-cli up

# Add engine as peer (engine listens on 127.0.0.1:51821 inside same container)
# Use engine's derived tunnel IP (not hardcoded 10.200.0.1)
wg set wg-ctl-cli peer "$ENGINE_PUB_KEY" allowed-ips ${ENGINE_TUNNEL_IP}/32 endpoint 127.0.0.1:51821
echo "CLI control tunnel ready."

# Wait for the engine process (keeps the container running)
wait $ENGINE_PID
