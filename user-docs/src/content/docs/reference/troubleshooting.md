---
title: Troubleshooting
description: Solutions for common issues.
sidebar:
  order: 2
---

## Engine

### Etcd connection issues

**Managed etcd**: If etcd fails to start, check that port 2379 is not already in use and that the data directory (`/var/lib/banyan/etcd/` by default) is writable.

**"failed to connect to etcd"** — For external etcd, ensure your etcd server is running and reachable at the configured address:

```bash
sudo apt-get install etcd-server    # Debian/Ubuntu
sudo systemctl start etcd
```

If you configured TLS or mTLS, verify that the certificate paths in `/etc/banyan/banyan.yaml` are correct and the files are readable.

To reconfigure the etcd connection, re-run `banyan-engine init`. See [Etcd](/getting-started/installation/#etcd-state-store) for setup details.

### Engine starts but agents cannot connect

Agents connect to the Engine's gRPC port (default: 50051). Check:

1. The Engine is running and the gRPC server started successfully (look for "Engine gRPC server listening on :50051" in the output).

2. The agent's config has the correct engine host and port. Check `/etc/banyan/banyan.yaml` on the worker:
   ```yaml
   agent:
     engine_host: <engine-ip>
     engine_port: "50051"
   ```

3. Port 50051 is open in your firewall between workers and the engine.

4. The agent and engine have the same cluster password.

### "VPC initialization: failed to write Flannel config"

This warning about `etcdctl` not being found is safe to ignore. It does not affect deployment functionality.

---

## Agent

### "nerdctl not found"

Install nerdctl on the worker node:

```bash
curl -L https://github.com/containerd/nerdctl/releases/download/v2.0.3/nerdctl-2.0.3-linux-amd64.tar.gz \
  | sudo tar -xz -C /usr/local/bin nerdctl
```

### "containerd not running"

Start containerd:

```bash
sudo systemctl start containerd
```

If containerd is not installed:

```bash
sudo apt-get install containerd
```

### Agent shows "ready" but tasks fail

Check if the Agent can pull images. SSH into the worker and test:

```bash
sudo nerdctl pull nginx:alpine
```

If this fails, the worker may not have internet access or the image registry may be unreachable.

---

## Deployment

### Deployment stays in "deploying" status

The Engine is waiting for Agents to complete their tasks. Check:

1. Are agents connected? Run `banyan-cli status`.
2. Check agent logs for errors in the terminal where `agent start` is running.
3. Verify agents can pull the images specified in your manifest.

### Deployment fails immediately

Check the error message in `banyan-cli status`. Common causes:

- **Image not found**: The image name in `banyan.yaml` is wrong or the registry is unreachable from workers.
- **Port conflict**: Another container is already using the same host port.

### "deployment timed out"

The deploy command waits up to 2 minutes by default. If your images are large, they may take longer to pull. Use `--no-wait` and check status manually:

```bash
banyan-cli deploy -f banyan.yaml --no-wait
# Check later:
banyan-cli status
```

### Containers are running but the application doesn't work

Banyan deploys containers but does not manage application-level networking between services across nodes. Containers on the same worker can communicate via localhost. Containers on different workers need external networking or a load balancer.

---

## General

### Permission errors

Engine and Agent commands need root access because they manage system services (data store, containerd):

```bash
sudo banyan-engine start
sudo banyan-agent start --node-name <name>
```

The `banyan-cli deploy` and `banyan-cli status` commands do not require root (but `banyan-cli init` does, to write `/etc/banyan/banyan.yaml`).

### Checking logs

Engine and Agent run in the foreground and print logs to stdout. Check the terminal where they are running.

Etcd logs:
- Managed etcd: Logs are printed to stdout alongside the engine output.
- External etcd: Check the logs of your externally managed etcd service.

### Stopping containers manually

If you need to remove containers directly on a worker:

```bash
sudo nerdctl rm -f <container-name>
```

To list all Banyan-managed containers:

```bash
sudo nerdctl ps | grep <app-name>
```
