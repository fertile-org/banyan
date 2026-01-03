#!/bin/bash
set -e

echo "========================================="
echo "Banyan Engine Container"
echo "========================================="

# Initialize engine
echo "Initializing engine..."
banyan-cli engine init

# Start engine (this will also start etcd)
echo "Starting engine..."
exec banyan-cli engine start --etcd-client-urls http://0.0.0.0:2379
