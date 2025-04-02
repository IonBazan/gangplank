# Gangplank

Gangplank is a CLI tool to manage UPnP port mappings, designed with Docker in mind. 
It fetches port mappings from Docker containers or YAML files and forwards them to your router via UPnP, making it a perfect fit for homelabs and self-hosted environments. 

Whether you’re running media servers, game servers, or development setups behind a NAT, Gangplank simplifies exposing services to your local network or the world.

## Why Gangplank?

Homelabs and self-hosted setups often run multiple services on a single machine, hidden behind a router’s NAT. Gangplank automates port forwarding, saving you from manually configuring your router. It’s especially useful for:
- **Dynamic Environments**: Docker containers start and stop, and Gangplank keeps ports in sync.
- **Ease of Use**: No need to log into your router’s admin page—Gangplank does it via UPnP.
- **Self-Hosted Accessibility**: Expose services like Plex, Nextcloud, or game servers to friends or the internet without static IPs or complex NAT rules.

### Homelab Examples
1. **Plex Media Server**  
   Expose Plex running in Docker to your local network or friends outside your NAT:
   ```bash
   docker run -d --network host --restart unless-stopped ionbazan/gangplank:latest daemon --refresh-interval 15m
   ```
   Assuming Plex uses port 32400, Gangplank forwards it automatically from your Docker setup.

2. **Minecraft Server**  
   Host a Minecraft server and let friends join from outside your network:
   ```bash
   docker run --rm --network host ionbazan/gangplank:latest add --external 25565 --internal 25565 --protocol TCP --name minecraft
   ```
   Or use `daemon` to keep it refreshed and poll Docker events.

3. **Development Web Server**  
   Test a web app locally and share it with a colleague over the internet:
   ```bash
   docker run --rm --network host ionbazan/gangplank:latest add --external 8080 --internal 80 --protocol TCP --name dev-web
   ```

## Features
- Fetch port mappings from Docker containers or YAML files.
- Forward ports via UPnP to your router.
- Poll Docker events to dynamically add/remove mappings (`daemon --poll`).
- Periodically refresh mappings to prevent expiration (`daemon` with `--refresh-interval`).
- Manually add or delete individual port mappings.

## Installation

Gangplank is distributed as a Docker image, ideal for homelabbers and self-hosters using Docker.

### Prerequisites
- Docker installed on your system.
- A UPnP-enabled router.

### Pull the Image
```bash
docker pull ionbazan/gangplank:latest
```

## Usage

Gangplank runs inside a Docker container. Here’s how to use it with `docker run` and `docker-compose`.

### Using `docker run`

#### Forward Initial Ports and Exit
Forward ports from running Docker containers (e.g., a homelab NAS or game server):
```bash
docker run --rm --network host ionbazan/gangplank:latest forward
```

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

### Using `docker-compose`

Set up Gangplank as a persistent service in your homelab with `docker-compose.yml`:

```yaml
services:
  gangplank:
    image: ionbazan/gangplank:latest
    network_mode: host
    restart: unless-stopped
    command: daemon --poll --refresh-interval 15m
```

Deploy it:
```bash
docker-compose up -d
```

Stop it:
```bash
docker-compose down
```

### Configuration Options
- `--cleanup-on-stop`: Deletes mappings when containers stop (use with `daemon --poll`).
- `--local-ip`: Overrides the local IP (e.g., `--local-ip 192.168.1.100` for a specific homelab machine).
- `--gateway`: Specifies the UPnP gateway URL (e.g., `--gateway http://192.168.1.1:49000/igd.xml`).

### Example with YAML for Static Services
Mount a `config.yaml` to forward fixed ports (e.g., a self-hosted VPN):
```bash
docker run --rm --network host -v ./config.yaml:/config.yaml ionbazan/gangplank:latest forward
```

Sample `config.yaml`:
```yaml
ports:
  - externalPort: 1194
    internalPort: 1194
    protocol: UDP
    name: openvpn
```

## Notes
- Gangplank uses a 1-hour lease duration for UPnP mappings. In `daemon` mode, `--refresh-interval` (default 15m) renews them before expiration.
- Use `--network host` for UPnP to reach your router; Docker’s bridge network won’t work for homelab NAT traversal.

## Contributing
Open issues or PRs on [GitHub](https://github.com/ionbazan/gangplank)!

## License
MIT

