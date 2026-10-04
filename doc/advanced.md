# Advanced usage

## Options

These options work with every command:

| Option | Description |
|---|---|
| `--config`, `-c` | Path to the YAML config file. By default, `config.yaml` in the working directory is used if it exists. |
| `--local-ip` | IP address that mappings point to. By default, Gangplank uses the address of the network interface that leads to your router. |
| `--gateway` | URL of your router's UPnP description, for example `http://192.168.1.1:5000/rootDesc.xml`. By default, Gangplank finds the router on its own. |
| `--ttl` | How long each mapping lasts before it expires. Default: `1h`. If your router only accepts permanent mappings, Gangplank notices and switches to them. |
| `--dry-run` | Don't talk to the router. Only print what would be done. |
| `--log-level` | How much to log: `debug`, `info` (default), `warn` or `error`. |
| `--log-format` | `text` (default) or `json`, for log collectors. |

These options are for the `daemon` command:

| Option | Description |
|---|---|
| `--poll`, `-p` | Watch Docker events and open ports as soon as containers start. The Docker image turns this on by default. |
| `--cleanup-on-stop` | Close a container's ports when it stops. |
| `--cleanup-on-exit` | Close all ports opened by Gangplank when it shuts down, for example on `docker stop`. |
| `--prune` | On every refresh, close Gangplank ports for this host that are no longer needed, for example for containers removed while Gangplank was not running. |
| `--refresh-interval` | How often mappings are renewed. Default: `15m`. Keep it shorter than `--ttl`. |

If the router can't be reached when the daemon starts (for example after a power cut, while the router is still booting), the daemon keeps trying every 30 seconds.
If Docker restarts, the daemon reconnects on its own.

## Environment variables

Every option can also be set with an environment variable. Use the option name in upper case, with `GANGPLANK_` in front and `_` instead of `-`:

```bash
GANGPLANK_REFRESH_INTERVAL=5m
GANGPLANK_POLL=true
GANGPLANK_GATEWAY=http://192.168.1.1:5000/rootDesc.xml
```

The usual Docker variables (`DOCKER_HOST`, `DOCKER_TLS_VERIFY`, `DOCKER_CERT_PATH`) work too, so Gangplank can use rootless Docker, Podman or a socket proxy.

## YAML config

The YAML file can hold settings as well as static ports:

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

There is a full [example file](../config.example.yaml), and the [usage guide](usage.md#static-ports-from-a-yaml-file) explains the `ports` list.

When a setting is given in more than one place, this order wins: command-line option, then environment variable, then YAML file, then the default.

## Security

- Access to the Docker socket is the same as root access on the host. A safer setup is a read-only socket proxy like [docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) with only `CONTAINERS=1` and `EVENTS=1`. Point Gangplank to it with `DOCKER_HOST=tcp://socket-proxy:2375`.
- Every port you label can be reached from the internet. Only expose services that are meant to be public, and keep them up to date.

## Troubleshooting

**"No UPnP IGD found"**
Check that UPnP is turned on in your router, and that Gangplank runs with `--network host`. On a Docker bridge network it can't find the router.

**Mappings point to the wrong IP address**
Set `--local-ip` to your host's LAN address.

**Ports work at home but not from outside**
Your router may itself be behind another router or your provider's NAT (double NAT or CGNAT). Compare the external address shown by your router with the one shown by a "what is my IP" website. If they differ, port forwarding on your router alone won't be enough.

**Finding the router takes long, or the wrong device answers**
Set `--gateway` to your router's UPnP description URL. Tools like `upnpc -l` (from miniupnpc) can show it.

## Commands

Besides the daemon, Gangplank has commands for one-off changes. They exit with an error code when something fails.

### Forward once and exit

Forward the ports of all running containers and the YAML file once:

```bash
docker run --rm --network host \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    ionbazan/gangplank:latest forward
```

Mappings expire after the TTL (1 hour by default), so run this regularly, for example from cron, or use the daemon.

### Run the daemon

Watch containers and renew mappings every 15 minutes:

```bash
docker run -d --network host --restart unless-stopped \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    ionbazan/gangplank:latest daemon --poll
```

Renew every 5 minutes and close ports when containers stop:

```bash
docker run -d --network host --restart unless-stopped \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    ionbazan/gangplank:latest daemon --poll --cleanup-on-stop --refresh-interval 5m
```

### List mappings

Show all mappings on the router, including ones made by other programs:

```bash
docker run --rm --network host ionbazan/gangplank:latest list
```

### Add a mapping

Open a port by hand. The format is `<external>:<internal>[/<protocol>]`:

```bash
docker run --rm --network host \
    ionbazan/gangplank:latest add 443:443/tcp --name nextcloud
```

### Delete a mapping

Close a port by hand, for example after a gaming session. The format is `<external>[/<protocol>]`:

```bash
docker run --rm --network host \
    ionbazan/gangplank:latest delete 25565/tcp
```
