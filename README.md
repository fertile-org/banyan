<div align="center">
  <img src="user-docs/src/assets/logo.png" alt="Banyan" width="240">
</div>

<h1 align="center">Banyan</h1>

<p align="center"><strong>Docker Compose syntax that scales.</strong></p>

<p align="center">
  <a href="https://github.com/fertile-org/banyan/actions/workflows/ci.yml"><img src="https://github.com/fertile-org/banyan/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://codecov.io/gh/fertile-org/banyan"><img src="https://codecov.io/gh/fertile-org/banyan/branch/main/graph/badge.svg" alt="Coverage"></a>
  <a href="https://github.com/fertile-org/banyan/releases/latest"><img src="https://img.shields.io/github/v/release/fertile-org/banyan?label=release" alt="Release"></a>
  <img src="https://img.shields.io/badge/go-1.24-00ADD8?logo=go" alt="Go 1.24">
</p>

<p align="center">Deploy containers across multiple servers with a YAML file you already know how to write.</p>

<p align="center">
  <a href="https://getbanyan.dev/">Documentation</a> &middot;
  <a href="https://getbanyan.dev/getting-started/quickstart/">Quickstart</a> &middot;
  <a href="https://getbanyan.dev/roadmap/">Roadmap</a> &middot;
  <a href="./DEVELOPMENT.md">Development</a>
</p>

---

> **Under experiment.** Banyan is not yet production-ready. We encourage you to experiment, break things, and [share feedback](https://github.com/fertile-org/banyan/issues).

## From one server to many

You know Docker Compose. You write a `docker-compose.yml`, run `docker compose up`, and everything works on one machine.

Then your app grows. You want your services spread across separate servers, or running multiple replicas to handle load. Either way, you need more than one machine.

**Banyan makes that step simple.** Use the same YAML syntax you already know, and Banyan distributes your services across your servers.

## Same syntax, more servers

**docker-compose.yml** — everything on one machine:

```yaml
services:
  web:
    build: ./web
    ports:
      - "80:80"

  api:
    build: ./api
    ports:
      - "8080:8080"
    environment:
      - DB_HOST=db

  db:
    image: postgres:15-alpine
```

**banyan.yaml** — distributed across your cluster:

```yaml
name: my-app

services:
  web:
    build: ./web
    ports:
      - "80:80"

  api:
    build: ./api
    deploy:
      replicas: 3
    ports:
      - "8080:8080"
    environment:
      - DB_HOST=db

  db:
    image: postgres:15-alpine
```

Same `services`. Same `build`. Same `ports`. Same `environment`. Add `name:` and `deploy.replicas`, and Banyan spreads them across your servers automatically.

## Features

- **Familiar syntax** — If you can write a docker-compose.yml, you can write a banyan.yaml. Same fields, same structure.
- **Single-binary components** — Three small, focused binaries. Download, run, done. No complex setup or configuration management required.
- **Built-in image registry** — No Docker Hub, no private registry setup. Use `build:` in your manifest and Banyan builds, stores, and distributes your images automatically. Deploy to a cluster as easily as running locally.
- **Automatic distribution** — Services are automatically distributed across your servers. Add a node, it picks up work on the next deployment.
- **Built-in VPC** — Secure cross-node networking using Flannel with VXLAN overlay and built-in DNS service discovery. Services on different servers communicate as if they were on the same network.
- **Proven foundations** — Built on battle-tested technologies: etcd, containerd, gRPC, and Prometheus metrics (coming soon).

## Is Banyan right for you?

Banyan is built for teams who:

- **Know Docker Compose** and want the same simplicity across multiple servers
- **Need to scale beyond one machine** without learning Kubernetes
- **Value shipping software** over operating infrastructure

Banyan bridges the gap between "docker compose up" and production orchestration — same syntax, distributed execution.

## Documentation

Full documentation is available at **[getbanyan.dev](https://getbanyan.dev/)**.

- [Installation](https://getbanyan.dev/getting-started/installation/)
- [Quickstart](https://getbanyan.dev/getting-started/quickstart/)
- [Manifest Reference](https://getbanyan.dev/guides/manifest-reference/)
- [Multi-Node Setup](https://getbanyan.dev/guides/multi-node/)
- [CLI Reference](https://getbanyan.dev/reference/cli/)
- [Troubleshooting](https://getbanyan.dev/reference/troubleshooting/)

## Install

```bash
# Engine node (control plane)
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role engine

# Worker nodes
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role agent
```

Or [build from source](https://getbanyan.dev/getting-started/installation/).

## Three commands to a running cluster

```bash
# On your control plane server
sudo banyan-engine start

# On each worker server
sudo banyan-agent start --node-name agent-1

# From anywhere
banyan-cli deploy -f banyan.yaml
```

Three focused binaries: `banyan-engine` for the control plane, `banyan-agent` for workers, `banyan-cli` for deployments.

## Architecture

```mermaid
graph TD
    CLI[fa:fa-terminal banyan-cli] -->|gRPC| Engine

    subgraph Engine[fa:fa-server banyan-engine]
        Store[(fa:fa-database etcd)]
        Registry[fa:fa-box-open Image Registry]
    end

    Engine -->|gRPC| Agent1
    Engine -->|gRPC| Agent2
    Engine -->|gRPC| AgentN

    subgraph VPC[fa:fa-network-wired Banyan VPC]
        subgraph A1[Worker 1]
            Agent1[fa:fa-cube banyan-agent]
            C1{{fa:fa-box container: web-0}}
            C2{{fa:fa-box container: api-0}}
            Agent1 ~~~ C1
            Agent1 ~~~ C2
        end

        subgraph A2[Worker 2]
            Agent2[fa:fa-cube banyan-agent]
            C3{{fa:fa-box container: api-1}}
            C4{{fa:fa-box container: db-0}}
            Agent2 ~~~ C3
            Agent2 ~~~ C4
        end

        subgraph AN[Worker N]
            AgentN[fa:fa-cube banyan-agent]
            C5{{fa:fa-box container: api-2}}
            AgentN ~~~ C5
        end
    end

    CLI ~~~ Prom(fa:fa-chart-line Prometheus-compatible)
    Engine -.-|/metrics| Prom
```

**CLI** sends commands to the **Engine** (control plane), which stores state in **etcd** and schedules work across **Agents** (workers). All communication over gRPC with password auth. Metrics are exposed in Prometheus format for monitoring.

## Roadmap

See the [Roadmap](https://getbanyan.dev/roadmap/) for what's next — metrics, auto-scaling, monitoring, and more.

## Contributing

See the [Development Guide](./DEVELOPMENT.md) for project structure, build commands, and architecture.

## License

Apache License 2.0. See [LICENSE](./LICENSE) for details.
