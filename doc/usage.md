# Usage examples

You choose what Gangplank forwards with labels on your containers, or with a YAML file for services that don't run in Docker.

The examples below assume Gangplank is already running as a daemon and your Docker host has the IP address `192.168.1.10`.

## How forwarding works

Your router sends traffic from an external port to a port on your Docker host. Docker then passes it on to the container.
So the "internal port" in a router mapping is always the port **on the host**, not the port inside the container.

## Forward all published ports

`gangplank.forward: "published"` forwards every port the container publishes on the host, using the same port number outside:

```yaml
services:
  nginx:
    image: nginx
    ports:
      - "80:80"
      - "8443:443"
    labels:
      gangplank.forward: "published"
```

Router mappings:

```
External 80   -> 192.168.1.10:80   (TCP)
External 8443 -> 192.168.1.10:8443 (TCP)
```

Ports published only on a loopback address, like `127.0.0.1:9000:9000`, are skipped. The router can't reach them anyway.

## Forward specific host ports

You can also give `gangplank.forward` a comma-separated list of ports.
Each entry is `<external>:<host port>`, or just `<port>` when both are the same. Add `/udp` for UDP; TCP is the default.

```yaml
services:
  nginx:
    image: nginx
    ports:
      - "80:80"
      - "443:443"
    labels:
      gangplank.forward: "443"
```

Router mapping:

```
External 443 -> 192.168.1.10:443 (TCP)
```

A game server with a TCP and a UDP port could use `gangplank.forward: "25565, 19132/udp"`.

## Forward container ports on random host ports

When you let Docker pick the host port (for example `ports: ["80"]`), you don't know the host port in advance.
Use `gangplank.forward.container` instead. It takes **container** ports, and Gangplank looks up which host port Docker picked.

Each entry is `[<external>:]<container port>[/<protocol>]`. Without an external port, the container port number is used.

```yaml
services:
  nginx:
    image: nginx
    ports:
      - "80" # Docker picks a host port, for example 32768
    labels:
      gangplank.forward.container: "8080:80"
```

Router mapping, if Docker picked `32768`:

```
External 8080 -> 192.168.1.10:32768 (TCP)
```

With `gangplank.forward.container: "80"` the external port would be `80`.

If the container port is not published at all, Gangplank logs a warning and skips it.

## Containers using the host network

Containers with `network_mode: host` don't publish ports, so `published` and `gangplank.forward.container` find nothing.
List the ports with `gangplank.forward` instead, for example `gangplank.forward: "32400"`.

## Static ports from a YAML file

For services outside Docker, list the ports in a YAML file.
Gangplank reads `gangplank.yaml` from its working directory (`/app/gangplank.yaml` in the image), or the file given with `--config`.

```yaml
ports:
  - externalPort: 80
    internalPort: 80
    name: nginx HTTP
  - externalPort: 51820
    internalPort: 51820
    protocol: UDP
    name: WireGuard
```

`protocol` is optional and defaults to TCP.

Mount the file into the container:

```bash
docker run -d --network host --restart unless-stopped \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    -v ./gangplank.yaml:/app/gangplank.yaml:ro \
    ionbazan/gangplank:latest
```

These ports are forwarded together with the ones from your containers.
If an entry is invalid, Gangplank logs an error and skips only that entry.

## When two services want the same port

If two sources ask for the same external port and protocol, the first one wins and Gangplank logs a warning.
The YAML file comes first, then containers sorted by name.

## More

See the [advanced guide](advanced.md) for all options, environment variables and commands.
