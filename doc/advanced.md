## Advanced usage

Gangplank can be used in various ways to suit your needs. Here are some advanced usage examples:

### Configuration Options

Gangplank can be configured using command-line options. Here are some of the most useful ones:

- `--poll`: Polls Docker events to dynamically add/remove mappings as containers start/stop.
- `--cleanup-on-stop`: Deletes mappings when containers stop (use with `daemon --poll`).
- `--local-ip`: Overrides the local IP (e.g., `--local-ip 192.168.1.100` for a specific homelab machine).
- `--gateway`: Specifies the UPnP gateway URL (e.g., `--gateway http://192.168.1.1:49000/igd.xml`).
- `--refresh-interval`: Sets the refresh interval for UPnP mappings (default is 15 minutes, e.g., `--refresh-interval 5m`).
- `--ttl`: Sets the time-to-live for UPnP mappings (default is 1 hour, e.g., `--ttl 30m`).
- `--dry-run`: Uses a dummy UPnP gateway for testing without making actual changes.

### Environment variables

You can also configure Gangplank using environment variables. Their names are prefixed with `GANGPLANK_` and follow the same naming convention as the command-line options. 
For example:

```bash
GANGPLANK_REFRESH_INTERVAL=5m
GANGPLANK_POLL=true
GANGPLANK_GATEWAY=192.168.0.1
```

### YAML Configuration

You can also use a YAML file to configure Gangplank. 
This is useful for more complex setups or when you want to manage multiple mappings in one place.

Naming is similar to command-line options - you can check out the [YAML config example](../config.example.yaml) for more details.

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
