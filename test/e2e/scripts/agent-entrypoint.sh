#!/bin/bash
set -e

echo "========================================="
echo "Banyan Agent Container"
echo "========================================="

# Get node name from environment or hostname
NODE_NAME=${NODE_NAME:-$(hostname)}
ENGINE_HOST=${ENGINE_HOST:-engine}
ENGINE_GRPC_PORT=${ENGINE_GRPC_PORT:-50051}

# 1. System setup (replicates banyan-agent init without interactive prompts)
echo "Setting up system..."
mkdir -p /etc/banyan/keys /var/lib/banyan/containers /etc/cni/net.d /opt/cni/bin /var/run
sysctl -w net.ipv4.ip_forward=1

# 2. Generate WireGuard keypair
echo "Generating WireGuard keypair..."
AGENT_PRIV_KEY=$(wg genkey)
AGENT_PUB_KEY=$(echo "$AGENT_PRIV_KEY" | wg pubkey)
echo "$AGENT_PRIV_KEY" > /etc/banyan/keys/agent.key
chmod 600 /etc/banyan/keys/agent.key
echo "Agent public key ($NODE_NAME): $AGENT_PUB_KEY"

# 3. Export agent public key for engine to whitelist
echo "$AGENT_PUB_KEY" > /tmp/keys-exchange/${NODE_NAME}.pub

# 4. Wait for engine public key (engine writes this before waiting for worker keys)
echo "Waiting for engine public key..."
while [ ! -f /tmp/keys-exchange/engine.pub ]; do
    sleep 1
done
ENGINE_WG_PUB_KEY=$(cat /tmp/keys-exchange/engine.pub)
echo "Engine WG public key: $ENGINE_WG_PUB_KEY"

# 5. Write complete config
echo "Writing config..."
cat > /etc/banyan/banyan.yaml <<EOF
agent:
    engine_host: ${ENGINE_HOST}
    engine_port: "${ENGINE_GRPC_PORT}"
    agent_name: ${NODE_NAME}
    wg_private_key_file: /etc/banyan/keys/agent.key
    wg_public_key: ${AGENT_PUB_KEY}
    engine_wg_public_key: ${ENGINE_WG_PUB_KEY}
EOF
chmod 600 /etc/banyan/banyan.yaml

# 6. Wait for engine to be ready (engine writes this file after gRPC is listening)
echo "Waiting for engine to be ready..."
while [ ! -f /tmp/keys-exchange/engine-ready ]; do
    sleep 1
done

# 7. Wait for engine gRPC to be reachable from this container
echo "Waiting for engine gRPC at ${ENGINE_HOST}:${ENGINE_GRPC_PORT}..."
until nc -z "${ENGINE_HOST}" "${ENGINE_GRPC_PORT}" 2>/dev/null; do
    sleep 1
done
echo "Engine is ready!"

# 8. Start containerd in background
echo "Starting containerd..."
containerd &
sleep 2

# 9. Start agent
echo "Starting agent..."
exec banyan-agent start --agent-name "$NODE_NAME"
