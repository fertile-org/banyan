---
title: Troubleshooting
description: Solutions for common issues.
sidebar:
  order: 2
---

## Engine

### "etcd not found"

The Engine requires etcd to be installed.

```bash
sudo apt-get install etcd-server    # Debian/Ubuntu
```

### Engine starts but agents cannot connect

Make sure etcd listens on all interfaces, not just localhost:

```bash
sudo banyan-cli engine start --etcd-client-urls http://0.0.0.0:2379
```

Verify from the worker machine:

```bash
curl http://<engine-ip>:2379/health
```

If this fails, check that port 2379 is open in your firewall.

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

1. Are agents connected? Run `banyan-cli engine status`.
2. Check agent logs for errors in the terminal where `agent start` is running.
3. Verify agents can pull the images specified in your manifest.

### Deployment fails immediately

Check the error message in `engine status`. Common causes:

- **Image not found**: The image name in `banyan.yaml` is wrong or the registry is unreachable from workers.
- **Port conflict**: Another container is already using the same host port.

### "deployment timed out"

The deploy command waits up to 2 minutes by default. If your images are large, they may take longer to pull. Use `--no-wait` and check status manually:

```bash
banyan-cli deploy -f banyan.yaml --no-wait
# Check later:
banyan-cli engine status
```

### Containers are running but the application doesn't work

Banyan deploys containers but does not manage application-level networking between services across nodes. Containers on the same worker can communicate via localhost. Containers on different workers need external networking or a load balancer.

---

## General

### Permission errors

Engine and Agent commands need root access because they manage system services (etcd, containerd):

```bash
sudo banyan-cli engine start
sudo banyan-cli agent start --node-name <name>
```

The `deploy` and `engine status` commands do not require root.

### Checking logs

Engine and Agent run in the foreground and print logs to stdout. Check the terminal where they are running.

Etcd logs are written to the file specified by `--etcd-log-file` (default: `/var/log/banyan-etcd.log`).

### Stopping containers manually

If you need to remove containers directly on a worker:

```bash
sudo nerdctl rm -f <container-name>
```

To list all Banyan-managed containers:

```bash
sudo nerdctl ps | grep <app-name>
```
