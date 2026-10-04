## Usage examples

You can control which ports are exposed and how they are mapped using labels in your Docker containers or by specifying them in a YAML file.

Let's look at some examples. Assuming you have a Gangplank container already running as daemon and your host machine IP is `192.168.1.10`.

### Expose all ports from a container

Using `gangplank.forward="published"` will expose all published ports of the container to the world on the same port numbers as on the host:

```yaml
services:
  nginx:
    image: nginx
    ports:
     - "80:80"
     - "8443:443"
    labels:
      gangplank.forward: "published" # Expose host ports 80 and 8443 to the world
```

Following UPnP rules will be created:
```
- ExternalPort=80, InternalPort=80, Protocol=TCP, InternalIP=192.168.1.10
- ExternalPort=8443, InternalPort=8443, Protocol=TCP, InternalIP=192.168.1.10
```

Ports published only on a loopback address (e.g. `127.0.0.1:9000:9000`) are skipped.

### Expose specific host ports

`gangplank.forward` also accepts a comma-separated list of `<external>:<host port>[/<protocol>]` entries (or just `<port>[/<protocol>]` when both are the same).
The internal port is the port **on the host** that Docker published, because that is where the router sends the traffic.
For example, to expose only port 443:

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

### Expose container ports published on random host ports

When Docker assigns a random host port (e.g. `ports: ["80"]`), use the `gangplank.forward.container` label.
It refers to **container** ports and Gangplank looks up the host port Docker bound them to.
The format is a comma-separated list of `[<external>:]<container port>[/<protocol>]`. When the external port is omitted, the container port number is used.

For example, to expose container port 80 on external port 8080:
```yaml
services:
  nginx:
    image: nginx
    ports:
     - "80" # Docker assigns a random host port (e.g. 32768)
    labels:
      gangplank.forward.container: "8080:80/tcp" # External 8080 -> container port 80
```

If Docker assigns host port `32768` to container port `80`, Gangplank creates the following UPnP rule:

```
- ExternalPort=8080, InternalPort=32768, Protocol=TCP, InternalIP=192.168.1.10
```

With `gangplank.forward.container: "80/tcp"` the external port would be `80` instead.

### Static port mapping

If you want to expose specific ports for services that are not running in Docker containers, you can set up static port mappings in a YAML file.
Gangplank reads `config.yaml` from its working directory (`/app/config.yaml` inside the container) or the file passed with `--config`:

```yaml
ports:
  - externalPort: 80
    internalPort: 80
    protocol: TCP # optional, defaults to TCP
    name: nginx HTTP
  - externalPort: 443
    internalPort: 443
    name: nginx HTTPS
```

```bash
docker run -d --network host --restart unless-stopped \
    -v /var/run/docker.sock:/var/run/docker.sock:ro \
    -v ./config.yaml:/app/config.yaml:ro \
    ionbazan/gangplank:latest
```

These ports are forwarded to the specified internal ports on your host machine, together with the ones discovered from Docker.
An invalid entry is reported and skipped without affecting the others.

If two sources request the same external port and protocol, the first one wins (the YAML file first, then containers sorted by name) and a warning is logged.

## Advanced Usage

You can find more advanced usage examples in the [advanced usage documentation](advanced.md).