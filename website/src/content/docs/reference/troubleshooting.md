---
title: Troubleshooting
description: Solutions for common issues.
sidebar:
  order: 2
---

## Engine

### Etcd connection issues

**Managed etcd**: If etcd fails to start, check that port 2379 is not already in use and that the data directory (`/var/lib/banyan/etcd/` by default) is writable.

**"failed to connect to etcd"** — For external etcd, make sure your etcd server is running and reachable at the configured address:

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
     wg_public_key: "<base64-key>"
   ```

3. Port 50051 is open in your firewall between workers and the engine.

4. The agent's public key is whitelisted on the engine. Check that a `.pub` file containing the agent's public key exists in `/etc/banyan/whitelisted-keys/` on the engine machine.

### "Unauthenticated" errors

If agents or CLI clients receive "Unauthenticated" errors:

1. Verify the component's public key is whitelisted on the engine. Check `/etc/banyan/whitelisted-keys/` for a `.pub` file containing the key.
2. If the engine was re-initialized, the whitelisted keys directory is recreated empty. Re-copy all agent and CLI public keys.
3. If no config exists yet, run `sudo banyan-agent init` (or `sudo banyan-cli init`) to generate a keypair, then whitelist the public key on the engine.
4. To find a component's public key: `grep wg_public_key /etc/banyan/banyan.yaml`

See [Authentication](/guides/authentication/) for details on key management.

### WireGuard overlay issues

If containers on different workers cannot communicate:

1. Check that `wireguard-tools` is installed on all workers: `wg --version`
2. Verify WireGuard kernel support: `ip link add wg-test type wireguard && ip link delete wg-test` — if this fails, the kernel module is missing (requires Linux 5.6+ or `wireguard-dkms`).
3. Ensure port 51820/UDP is open between workers.
4. If WireGuard is unavailable, Banyan falls back to VXLAN automatically. You can also force VXLAN by setting `overlay_type: "vxlan"` in the engine config.

### Control tunnel issues

If agents or CLI cannot connect through the WireGuard control tunnel:

1. Check that the `wg-control` interface exists: `ip link show wg-control`
2. Verify the tunnel peer: `wg show wg-control`
3. Ensure port 51821/UDP is open from agents/CLI to the engine.
4. Test connectivity: `ping 10.200.0.1` from the agent/CLI.
5. If the control tunnel fails, Banyan falls back to direct TCP with public key metadata authentication. Check the agent/engine logs for "Control tunnel setup failed" messages.
6. The CLI creates its tunnel during `banyan-cli init` (requires root). Subsequent CLI commands don't need root because the tunnel persists as a kernel interface.

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

1. Are agents connected? Run `banyan-cli agent`.
2. Check agent logs for errors in the terminal where `agent start` is running.
3. Verify agents can pull the images specified in your manifest.

### Deployment fails immediately

Check the error message in `banyan-cli deployment`. Common causes:

- **Image not found**: The image name in `banyan.yaml` is wrong or the registry is unreachable from workers.
- **Port conflict**: Another container is already using the same host port.

### "deployment timed out"

The `up` command waits up to 2 minutes by default. If your images are large, they may take longer to pull. Use `--no-wait` and check status manually:

```bash
banyan-cli up -f banyan.yaml --no-wait
# Check later:
banyan-cli deployment
```

### Redeployment doesn't replace old containers

When you run `banyan-cli up` again, Banyan should automatically replace old containers using a blue-green strategy. If old containers aren't being replaced:

1. Check that the application name in `banyan.yaml` matches the running deployment. The name must be identical for Banyan to recognize it as a redeployment.
2. If the old deployment is in `stopping` or `deploying` state, the Engine waits for it to finish before scheduling the new one. Check `banyan-cli deployment` and wait a few seconds.
3. If a previous redeployment failed, the old containers stay running. Fix the issue and run `banyan-cli up` again — it will retry the replacement.

### Old containers still running after redeployment

During blue-green redeployment, old containers run alongside new ones until the new deployment is confirmed healthy. This overlap is expected and usually lasts a few seconds. If old containers persist:

1. The new deployment may have failed. Check `banyan-cli deployment` for the deployment status and error message.
2. If the new deployment failed, old containers are intentionally kept running to avoid downtime. Fix the issue and redeploy.

See [Redeployment](/guides/redeployment/) for details on how blue-green and per-service deploys work.

### Per-service deploy fails with dependency error

When deploying specific services (e.g., `banyan-cli up -f banyan.yaml web`), Banyan validates that all `depends_on` dependencies are satisfied. If you see an error like:

```
Error: service "web" depends on "api" which is not running and not being deployed
```

Either deploy the dependency too (`banyan-cli up -f banyan.yaml web api`) or make sure the dependency is already running in the existing deployment.

### Containers are running but the application doesn't work

Banyan deploys containers but does not manage application-level networking between services across nodes. Containers on the same worker can communicate via localhost. Containers on different workers need external networking or a load balancer.

---

## General

### Permission errors

The engine and agent require `sudo` for all commands — they manage network interfaces, iptables rules, and containers:

```bash
sudo banyan-engine init
sudo systemctl enable --now banyan-engine

sudo banyan-agent init
sudo systemctl enable --now banyan-agent
```

The CLI only needs `sudo` for `init` (to create the WireGuard control tunnel). All other CLI commands (`up`, `down`, `engine`, `agent`, `deployment`, `container`, `events`, `logs`, `dashboard`) run as your normal user.

### Checking logs

When running as a systemd service, use `journalctl`:

```bash
sudo journalctl -u banyan-engine -f   # engine logs
sudo journalctl -u banyan-agent -f    # agent logs
```

When running in the foreground (`sudo banyan-engine start`), logs print to stdout.

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
