## Advanced usage

Gangplank can be used in various ways to suit your needs. Here are some advanced usage examples:

### Configuration Options

Gangplank can be configured using command-line options. Global options:

- `--config`, `-c`: Path to the YAML config file (default: `config.yaml` in the working directory, if present).
- `--local-ip`: Overrides the local IP mappings point to (e.g., `--local-ip 192.168.1.100`). By default, Gangplank uses the address of the interface that routes to the gateway.
- `--gateway`: The UPnP gateway description URL (e.g., `--gateway http://192.168.1.1:5000/rootDesc.xml`). By default, the gateway is discovered via SSDP.
- `--ttl`: Sets the lease duration of UPnP mappings (default is 1 hour, e.g., `--ttl 30m`). Gateways that only support permanent leases are detected automatically.
- `--dry-run`: Uses a dummy UPnP gateway for testing without making actual changes.

Options of the `daemon` command:

- `--poll`, `-p`: Listens to Docker events to add mappings as soon as containers start. The image enables it by default.
- `--cleanup-on-stop`: Deletes mappings when containers stop.
- `--cleanup-on-exit`: Deletes all mappings created by the daemon when it shuts down (SIGTERM/SIGINT, e.g. `docker stop`).
- `--prune`: On every refresh, deletes Gangplank mappings pointing to this host that are no longer wanted, e.g. for containers removed while Gangplank was not running.
- `--refresh-interval`: Sets the refresh interval for UPnP mappings (default is 15 minutes, e.g., `--refresh-interval 5m`). Keep it shorter than `--ttl`.

If the gateway cannot be reached when the daemon starts (e.g. the router is still booting), the daemon keeps retrying every 30 seconds.

### Environment variables

You can also configure Gangplank using environment variables. Their names are prefixed with `GANGPLANK_` and follow the same naming convention as the command-line options.
For example:

```bash
GANGPLANK_REFRESH_INTERVAL=5m
GANGPLANK_POLL=true
GANGPLANK_GATEWAY=http://192.168.0.1:5000/rootDesc.xml
```

The standard Docker variables (`DOCKER_HOST`, `DOCKER_TLS_VERIFY`, `DOCKER_CERT_PATH`) are honored, so Gangplank works with rootless Docker, Podman or a socket proxy.

### YAML Configuration

You can also use a YAML file to configure Gangplank.
This is useful for more complex setups or when you want to manage multiple mappings in one place.

```yaml
ttl: 60m
refreshInterval: 15m
gateway: http://192.168.1.1:5000/rootDesc.xml # optional
localIp: 192.168.1.10 # optional
ports:
  - externalPort: 8080
    internalPort: 80
    protocol: TCP
    name: web
```

See the [YAML config example](../config.example.yaml) and the [usage documentation](usage.md#static-port-mapping).

Settings are applied with the following precedence: command-line flag > environment variable > YAML file > default.

### Security

- Mounting the Docker socket gives Gangplank root-equivalent access to the host. Consider a read-only socket proxy such as [tecnativa/docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) with only `CONTAINERS=1` and `EVENTS=1` enabled, and point Gangplank to it with `DOCKER_HOST=tcp://socket-proxy:2375`.
- Every port you label is reachable from the internet. Only expose services that are meant to be public and keep them up to date.

### Troubleshooting

- **No UPnP IGD found**: make sure UPnP is enabled on your router and that Gangplank runs with `--network host`. Docker bridge networks do not receive SSDP discovery responses.
- **Mappings point to the wrong IP**: set `--local-ip` explicitly.
- **Mappings work on the LAN but not from the internet**: your router may be behind another NAT (double NAT or carrier-grade NAT). Compare the external IP reported by your router with the one shown by an online "what is my IP" service.
- **Discovery is slow or picks the wrong device**: set `--gateway` to the IGD description URL. You can find it with tools like `upnpc -l` (from miniupnpc).

## Commands

Besides daemon mode, Gangplank offers several commands to manage port mappings on an ad-hoc basis.

#### Forward Initial Ports and Exit

Forward ports from running Docker containers (e.g., a homelab NAS or game server):
```bash
docker run --rm --network host \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    ionbazan/gangplank:latest forward
```

Please note that because the default lease duration is 1 hour, you will need to run this command every hour to keep the mappings alive.
Consider adding it to your cron, or use the `daemon` mode instead.

#### Run as a Daemon for a Self-Hosted Setup

Keep ports open for a dynamic homelab, polling Docker events and refreshing every 15 minutes:
```bash
docker run -d --network host --restart unless-stopped \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    ionbazan/gangplank:latest daemon --poll
```

Customize the refresh interval (e.g., 5 minutes) and clean up after stopped containers:

```bash
docker run -d --network host --restart unless-stopped \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    ionbazan/gangplank:latest daemon --poll --cleanup-on-stop --refresh-interval 5m
```

#### List Port Mappings

Show all mappings currently configured on the gateway:

```bash
docker run --rm --network host ionbazan/gangplank:latest list
```

#### Add a Port for a Local Service

Expose a self-hosted service (e.g., Nextcloud) outside your NAT. The format is `<external>:<internal>[/<protocol>]`:
```bash
docker run --rm --network host \
    ionbazan/gangplank:latest add 443:443/tcp --name nextcloud
```

#### Delete a Port Mapping

Remove a mapping when you’re done (e.g., after a gaming session). The format is `<external>[/<protocol>]`:

```bash
docker run --rm --network host \
    ionbazan/gangplank:latest delete 25565/tcp
```

All commands exit with a non-zero status when the operation fails.
