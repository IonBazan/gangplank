![logo](logo.png)

# Gangplank

Gangplank is a CLI tool to manage UPnP port mappings, designed with Docker in mind. 
It automatically grabs port mappings from running Docker containers or YAML files and forwards them to your router via UPnP, making it a perfect fit for homelabs and self-hosted environments. 

Whether you’re running media servers, game servers, or development setups behind a NAT, Gangplank simplifies exposing services to your local network or WAN.

> **Gangplank** - (nautical) A movable board used to board or disembark from a ship, bridging the gap between vessel and shore

## Why Gangplank?

- **Dynamic Environments**: Docker containers start and stop, and Gangplank keeps ports in sync.
- **Ease of Use**: No need to log into your router’s admin page—Gangplank does it via UPnP.
- **Self-Hosted Accessibility**: Expose services like Plex, Nextcloud, or game servers to friends or the internet without static IPs or complex NAT rules.

## Getting started

Gangplank is distributed as a Docker image, ideal for homelabbers and self-hosters using Docker.

**Prerequisites:**

- Docker installed on your system.
- A UPnP-enabled router.

Pull the image:
```bash
docker pull ionbazan/gangplank:latest
```

Run Gangplank as a daemon (default mode):

```bash
docker run -d --network host \
    --restart unless-stopped \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    ionbazan/gangplank:latest
```

or add it to your `docker-compose.yml`:

```yaml
services:
  gangplank:
    image: ionbazan/gangplank
    network_mode: host
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    restart: unless-stopped
```

## Usage Examples

You can control which ports are exposed and how they are mapped using labels in your Docker containers or by specifying them in a YAML file.

Let's look at some examples. Let's assume you have a Gangplank container already running as daemon and your host machine IP is `192.168.1.10`.

### Expose all ports from a container

Using `gangplank.forward="published"` will expose all ports from the container to the world on the same port numbers:

```yaml
services:
  nginx:
    image: nginx
    ports:
     - "80:80"
     - "443:443"
    labels:
      gangplank.forward: "published" # Expose port 80 and 443 to the world
```

Following UPnP rules will be created:
```
- ExternalPort=80, InternalPort=80, Protocol=TCP, InternalIP=192.168.1.10
- ExternalPort=443, InternalPort=443, Protocol=TCP, InternalIP=192.168.1.10
```

### Expose specific port from a container

You can also specify which ports to expose using the `gangplank.forward` label. 
For example, if you want to expose only port 443 from a container:

```yaml
services:
  nginx:
    image: nginx
    ports:
     - "80:80"
     - "443:443"
    labels:
      gangplank.forward: "443:443/tcp" # Only expose port 443 to the world
```

Following UPnP rules will be created:

```
- ExternalPort=443, InternalPort=443, Protocol=TCP, InternalIP=192.168.1.10
```

### Expose Specific Random Port from a Container

When Docker assigns a random host port (e.g., `:<container_port>`), Gangplank can still expose it to a specific external port using the `gangplank.forward.container` label in the format `<external>:<internal>/<protocol>`.

For example, to expose container port 80 on external port 8080:
```yaml
services:
  nginx:
    image: nginx
    ports:
     - ":80" # Assigns a random host port (e.g., 32768)
    labels:
      gangplank.forward.container: "8080:80/tcp" # Maps external 8080 to container 80
```

If Docker assigns host port `32768` to container port `80`, Gangplank creates following UPnP rules:

```
- ExternalPort=80, InternalPort=32768, Protocol=TCP, InternalIP=192.168.1.10
```

### Static port mapping

If you want to expose specific ports for services that are not running in Docker containers, you can set up static port mappings using a YAML file located in `/app/config.yaml` inside the container.

```yaml
ports:
  - externalPort: 80
    internalPort: 80
    protocol: TCP
    name: nginx HTTP
  - externalPort: 443
    internalPort: 443
    protocol: TCP
    name: nginx HTTPS
```

These ports will be handled by Gangplank and forwarded to the specified internal ports on your host machine.

## Features
- Fetch port mappings from Docker containers or YAML files.
- Forward ports via UPnP to your router.
- Poll Docker events to dynamically add/remove mappings (`daemon --poll`).
- Periodically refresh mappings to prevent expiration (`daemon` with `--refresh-interval`).
- Manually add or delete individual port mappings.

### Configuration Options
- `--cleanup-on-stop`: Deletes mappings when containers stop (use with `daemon --poll`).
- `--local-ip`: Overrides the local IP (e.g., `--local-ip 192.168.1.100` for a specific homelab machine).
- `--gateway`: Specifies the UPnP gateway URL (e.g., `--gateway http://192.168.1.1:49000/igd.xml`).

## Commands

Besides of daemon mode, Gangplank offers several commands to manage port mappings on an ad-hoc basis.

#### Forward Initial Ports and Exit

Forward ports from running Docker containers (e.g., a homelab NAS or game server):
```bash
docker run --rm --network host ionbazan/gangplank:latest forward
```

Please note that because default rules TTL is 1 hour, you will need to run this command every hour to keep the mappings alive.
Consider adding it to your cron, or use the `daemon` mode instead.

#### Run as a Daemon for a Self-Hosted Setup

Keep ports open for a dynamic homelab, polling Docker events and refreshing every 15 minutes:
```bash
docker run -d --network host --restart unless-stopped ionbazan/gangplank:latest daemon --poll
```

Customize the refresh interval (e.g., 5 minutes):

```bash
docker run -d --network host --restart unless-stopped ionbazan/gangplank:latest daemon --poll --refresh-interval 5m
```

#### Add a Port for a Local Service

Expose a self-hosted service (e.g., Nextcloud) outside your NAT:
```bash
docker run --rm --network host ionbazan/gangplank:latest add --external 443 --internal 443 --protocol TCP --name nextcloud
```

#### Delete a Port Mapping

Remove a mapping when you’re done (e.g., after a gaming session):

```bash
docker run --rm --network host ionbazan/gangplank:latest delete --external 25565 --protocol TCP
```

## Notes
- Gangplank uses a 1-hour lease duration for UPnP mappings. In `daemon` mode, `--refresh-interval` (default 15m) renews them before expiration.
- Use `--network host` for UPnP to reach your router; Docker’s bridge network won’t work for homelab NAT traversal.

## Contributing
Open issues or PRs on [GitHub](https://github.com/ionbazan/gangplank)!

## License
MIT

## Similar Projects

There are existing projects that provide similar functionality, but they are either outdated or not actively maintained. 
Gangplank aims to fill this gap with a modern, Docker-centric approach to UPnP port mapping.

- https://github.com/danielbodart/portical
- https://github.com/ProjectInitiative/upnp-service

