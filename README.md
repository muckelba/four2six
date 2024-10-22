# Four2Six

A tool to forward IPv4 traffic to an IPv6 destination.

I've built this tool to solve a very specific problem. My ISP does not provide me with a dual stack internet connection so the only way to access my home network from the internet is by using IPv6. This tool runs on a cloud server and listens on every IPv4 request to my home network and forwards it to the IPv6 address at home. I've combined that with [cloudflare-ddns](https://github.com/favonia/cloudflare-ddns/) running at home to update my local IPv6 address and sending a webhook notification to this tool to update its target IPv6 address on the fly.

This is my personal setup using Four2Six:

![showcase](https://github.com/user-attachments/assets/d372d0d3-20ff-4600-af5a-b1517b67a701)

> The `AAAA` record of my home domain is pointing directly to my router. The `A` record is pointing to my cloud server that is hosting Four2Six and forwards every IPv4 traffic to my router via IPv6. Cloudflare-ddns is sending a webhook whenever the IPv6 address of my router changes.

## Features

- Update target IPv6 address with a webhook
- Monitor the target port health

## Configuration

Four2Six can be configured by using environment variables:

| env name              | default   | description                                                                            |
| --------------------- | --------- | -------------------------------------------------------------------------------------- |
| `DEST_PORTS`          | `8080`    | A list of comma separated port numbers to which Four2Six should forward the traffic to |
| `SRC_LISTEN_ADDR`     | `0.0.0.0` | On which interface Four2Six should listen for incoming traffic                         |
| `SRC_PORTS`           | `8080`    | A list of comma separated port numbers on which Four2Six should listen for traffic     |
| `WEBHOOK_LISTEN_ADDR` | `0.0.0.0` | On which interface Four2Six should listen for incoming HTTP requests                   |
| `WEBHOOK_LISTEN_PORT` | `8081`    | On which port Four2Six should listen for incoming HTTP requests                        |
| `WEBHOOK_TOKEN`       |           | **Required**, a string of characters that secures the `/update` HTTP endpoint          |

> [!IMPORTANT]  
> The order of the destination ports needs to be same as the source ports when forwarding multiple ports! E.g. when forwarding port 8080 to port 80 and port 7070 to port 443, make sure to set `DEST_PORTS` to `80,443` and `SRC_PORTS` to `8080,7070`.

### Target IPv6 Address

The target IPv6 address can be set dynamically hence why it's not set as an environment variable. It is stored in a simple txt file called `ipv6_address.txt` which resides within a `data/` directory (which gets automatically created if it's not existing yet).

> [!NOTE]  
> Four2Six is currently only writing to the file, not reading. So updating it manually has no effect.

To update the target IPv6 address, send a HTTP POST to the `/update` endpoint with an IPv6 address as payload. Originally, i wanted to expect a JSON payload but since cloudflare-ddns just sends some text without formatting etc, i've decided to ~~steal~~ add a regex expression that just parses the received text for an IPv6 address.

#### Example

```bash
curl 'http://localhost:8081/update' -H 'Authorization: Bearer DemoReplaceMe' -d 'This is a random text that contains a random IPv6 address somewhere: c995:8375:24d4:fec8:5bcb:f3ed:3d34:a2cd'
```

## Running

The preferred way to run Four2Six is by using Docker. You can always compile the [main.go](main.go) yourself and run it as a binary directly of course.

### Docker and IPv6

To enable IPv6 networking for a docker container, connect it to a IPv6 capable docker network. Make sure that your Docker host system is able to connect to IPv6 networks. Read more about it [here](https://docs.docker.com/engine/daemon/ipv6/).

### Docker run

Create a IPv6 docker network first and then start the container with a minimal config. This will forward any incoming traffic from port 8080 to port 8080.

```bash
docker network create --ipv6 --subnet 2001:db8::/64 ip6net
docker run -v "${PWD}/data:/app/data" --network ip6net -p 8080:8080 -p 8081:8081 -e WEBHOOK_TOKEN=DemoReplaceMe ghcr.io/muckelba/four2six:main
```

### Docker Compose

```yaml
services:
  four2six:
    image: ghcr.io/muckelba/four2six:main
    restart: unless-stopped
    networks:
      - default
    env_file:
      - .env
    volumes:
      - ./data:/app/data

networks:
  default:
    enable_ipv6: true
    ipam:
      config:
        - subnet: 2001:db8::/64
```

### Traefik

Here's an example config if you want to run this container behind [Traefik](https://traefik.io/) assuming the proxy network for traefik is called `traefik`, the HTTP endpoints should be reachable at `four2six.example.com` and the incoming traffic is pointed at `homelab.example.com` or `*.homelab.example.com`.

#### `.env`

```ini
WEBHOOK_TOKEN=DemoReplaceMe
SRC_PORTS=80,443
DEST_PORTS=80,443
```

#### `compose.yaml`

```yaml
services:
  four2six:
    image: ghcr.io/muckelba/four2six:main
    restart: unless-stopped
    networks:
      - default
      - traefik
    env_file:
      - .env
    volumes:
      - ./data:/app/data
    labels:
      - "traefik.enable=true"
      - "traefik.docker.network=traefik"

      # Webhook
      - "traefik.http.routers.four2six.rule=Host(`four2six.example.com`)"
      - "traefik.http.routers.four2six.entrypoints=https"
      - "traefik.http.routers.four2six.tls.certresolver=letsencrypt"
      - "traefik.http.routers.four2six.service=four2six"
      - "traefik.http.services.four2six.loadbalancer.server.port=8081"

      # HTTP Tunnel
      - "traefik.http.routers.four2six-http-tunnel.rule=HostRegexp(`^.*\\.*homelab\\.example\\.com$$`)"
      - "traefik.http.routers.four2six-http-tunnel.entrypoints=http"
      - "traefik.http.routers.four2six-http-tunnel.service=four2six-http-tunnel"
      - "traefik.http.services.four2six-http-tunnel.loadbalancer.server.port=80"

      # HTTPS Tunnel
      - "traefik.tcp.routers.four2six-https-tunnel.rule=HostSNIRegexp(`^.*\\.*homelab\\.example\\.com$$`)"
      - "traefik.tcp.routers.four2six-https-tunnel.entrypoints=https"
      - "traefik.tcp.routers.four2six-https-tunnel.service=four2six-https-tunnel"
      - "traefik.tcp.routers.four2six-https-tunnel.tls.passthrough=true"
      - "traefik.tcp.services.four2six-https-tunnel.loadbalancer.server.port=443"

networks:
  default:
    enable_ipv6: true
    ipam:
      config:
        - subnet: 2001:db8::/64
  traefik:
    external: true
```
