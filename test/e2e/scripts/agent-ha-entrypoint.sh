#!/bin/bash
set -e

echo "========================================="
echo "Banyan Agent HA Container (${NODE_NAME})"
echo "========================================="

NODE_NAME=${NODE_NAME:-$(hostname)}
ENGINE_ENDPOINTS=${ENGINE_ENDPOINTS:-172.28.0.10:50051,172.28.0.13:50051}

# 1. System setup
echo "Setting up system..."
mkdir -p /etc/banyan/keys /var/lib/banyan/containers /etc/cni/net.d /opt/cni/bin /var/run
sysctl -w net.ipv4.ip_forward=1

# 2. Generate WireGuard keypair (needed for agent auth)
echo "Generating WireGuard keypair..."
AGENT_PRIV_KEY=$(wg genkey)
AGENT_PUB_KEY=$(echo "$AGENT_PRIV_KEY" | wg pubkey)
echo "$AGENT_PRIV_KEY" > /etc/banyan/keys/agent.key
chmod 600 /etc/banyan/keys/agent.key

# 3. Signal that this worker is ready
echo "$AGENT_PUB_KEY" > /tmp/keys-exchange/${NODE_NAME}-ready
echo "${NODE_NAME} signaled ready (pubkey: ${AGENT_PUB_KEY})"

# 4. Write agent config — use single-engine backward-compat fields (no WG tunnel in E2E)
# The engine runs with --allow-insecure so WG is not required for connectivity
echo "Writing config..."
IFS=',' read -ra ENDPOINTS <<< "$ENGINE_ENDPOINTS"
PRIMARY_HOST=$(echo "${ENDPOINTS[0]}" | cut -d: -f1)
PRIMARY_PORT=$(echo "${ENDPOINTS[0]}" | cut -d: -f2)

cat > /etc/banyan/banyan.yaml <<EOF
agent:
    agent_name: ${NODE_NAME}
    engine_host: ${PRIMARY_HOST}
    engine_port: "${PRIMARY_PORT}"
    wg_public_key: ${AGENT_PUB_KEY}
EOF
chmod 600 /etc/banyan/banyan.yaml

echo "Config written with primary: ${PRIMARY_HOST}:${PRIMARY_PORT}"

# 5. Wait for at least one engine gRPC to be ready
echo "Waiting for engine gRPC..."
ENGINE_READY=false
for i in $(seq 1 120); do
    for ep in "${ENDPOINTS[@]}"; do
        HOST=$(echo "$ep" | cut -d: -f1)
        PORT=$(echo "$ep" | cut -d: -f2)
        if nc -z "$HOST" "$PORT" 2>/dev/null; then
            ENGINE_READY=true
            echo "Engine at $ep is ready."
            break 2
        fi
    done
    sleep 1
done

if [ "$ENGINE_READY" = false ]; then
    echo "ERROR: No engine became ready within timeout"
    exit 1
fi

# 6. Start containerd
echo "Starting containerd..."
containerd &
sleep 2

# 7. Start agent (engine runs with --allow-insecure, no WG tunnel needed in E2E)
echo "Starting agent..."
exec banyan-agent start --agent-name "$NODE_NAME"
