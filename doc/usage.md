## Usage examples

You can control which ports are exposed and how they are mapped using labels in your Docker containers or by specifying them in a YAML file.

Let's look at some examples. Assuming you have a Gangplank container already running as daemon and your host machine IP is `192.168.1.10`.

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

## Advanced Usage

You can find more advanced usage examples in the [advanced usage documentation](advanced.md).