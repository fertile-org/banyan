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
  <a href="https://feritle-banyan.vercel.app/">Documentation</a> &middot;
  <a href="https://feritle-banyan.vercel.app/getting-started/quickstart/">Quickstart</a> &middot;
  <a href="https://feritle-banyan.vercel.app/roadmap/">Roadmap</a> &middot;
  <a href="./DEVELOPMENT.md">Development</a>
</p>

---

> **Under heavy development.** Banyan is not yet production-ready. We encourage you to experiment, break things, and [share feedback](https://github.com/fertile-org/banyan/issues).

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
      - DB_PORT=5432

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
    depends_on:
      - api

  api:
    build: ./api
    deploy:
      replicas: 3
    ports:
      - "8080:8080"
    environment:
      - DB_HOST=my-app-db-0
      - DB_PORT=5432
    depends_on:
      - db

  db:
    image: postgres:15-alpine
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_USER=banyan
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=app
```

Same `services`. Same `image`. Same `ports`. Same `environment`. Banyan spreads them across your servers automatically — and `deploy.replicas` lets you run multiple copies when you need them.

## Install

```bash
# Engine node (control plane)
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role engine

# Worker nodes
curl -sSL https://raw.githubusercontent.com/fertile-org/banyan/main/install.sh | sudo bash -s -- --role agent
```

Or [build from source](https://feritle-banyan.vercel.app/getting-started/installation/).

## Three commands to a running cluster

```bash
# On your control plane server
sudo banyan-cli engine start

# On each worker server
sudo banyan-cli agent start --node-name agent-1

# From anywhere
banyan-cli deploy -f banyan.yaml
```

No package managers. No plugins. One binary does everything.

## Features

- **Familiar syntax** — If you can write a docker-compose.yml, you can write a banyan.yaml. Same fields, same structure.
- **Single binary** — `banyan-cli` is the Engine, the Agent, and the deploy tool. Build once, copy to your servers, done.
- **Built-in image registry** — No Docker Hub, no private registry setup. Use `build:` in your manifest and Banyan builds, stores, and distributes your images automatically. Deploy to a cluster as easily as running locally.
- **Automatic distribution** — Containers spread across workers with round-robin scheduling. Add a server, it joins the next deployment.
- **Proven foundations** — etcd for state coordination. containerd for running containers. No experimental runtimes.

## Is Banyan right for you?

Banyan is built for teams who:

- Deploy to 1–20 servers
- Know Docker Compose and want the same simplicity in production
- Value getting things running over configuring infrastructure

## Documentation

Full documentation is available at **[feritle-banyan.vercel.app](https://feritle-banyan.vercel.app/)**.

- [Installation](https://feritle-banyan.vercel.app/getting-started/installation/)
- [Quickstart](https://feritle-banyan.vercel.app/getting-started/quickstart/)
- [Manifest Reference](https://feritle-banyan.vercel.app/guides/manifest-reference/)
- [Multi-Node Setup](https://feritle-banyan.vercel.app/guides/multi-node/)
- [CLI Reference](https://feritle-banyan.vercel.app/reference/cli/)
- [Troubleshooting](https://feritle-banyan.vercel.app/reference/troubleshooting/)

## Roadmap

See the [Roadmap](https://feritle-banyan.vercel.app/roadmap/) for what's next — metrics, auto-scaling, monitoring, and more.

## Contributing

See the [Development Guide](./DEVELOPMENT.md) for project structure, build commands, and architecture.

## License

Apache License 2.0. See [LICENSE](./LICENSE) for details.
