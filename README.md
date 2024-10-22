# Four2Six

[![Go Version](https://img.shields.io/github/go-mod/go-version/muckelba/four2six)](https://go.dev)
[![License](https://img.shields.io/github/license/muckelba/four2six)](LICENSE)

Four2Six is a lightweight Go service that bridges IPv4 traffic to IPv6 destinations. It's useful for accessing IPv6-only networks from IPv4 clients (typically home networks with IPv6-only internet connections).

## 🎯 Use Case

I've built this tool to solve a very specific problem. My ISP does not provide me with a dual stack internet connection so the only way to access my home network from the internet is by using IPv6. This tool runs on a cloud server and listens on every IPv4 request to my home network and forwards it to the IPv6 address at home.

![Architecture Diagram](https://github.com/user-attachments/assets/d372d0d3-20ff-4600-af5a-b1517b67a701)

### Example Setup

- Your home router has a dynamic IPv6 address
- `AAAA` record points directly to your router
- `A` record points to your cloud server running Four2Six
- [cloudflare-ddns](https://github.com/favonia/cloudflare-ddns/) updates your IPv6 address and notifies Four2Six via a webhook (Set the `SHOUTRRR` environment to `generic+http://four2six.example.com/update?@Authorization=Bearer+your-token-here`)

## 🚀 Features

- **Dynamic IPv6 Forwarding**: Forward IPv4 traffic to any IPv6 destination
- **Multiple Port Mapping**: Configure multiple source-to-destination port mappings
- **Webhook Updates**: Update target IPv6 address dynamically via HTTP webhook
- **Health Monitoring**: Built-in health checks for all configured tunnels
- **Docker Support**: Ready-to-use Docker container with IPv6 networking
- **Traefik Integration**: Example configurations for Traefik reverse proxy

## 🛠️ Configuration

### Environment Variables

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `WEBHOOK_TOKEN` | - | ✅ | Authentication token for the `/update` endpoint |
| `DEST_PORTS` | `8080` | ❌ | Comma-separated list of destination ports |
| `SRC_PORTS` | `8080` | ❌ | Comma-separated list of source ports |
| `SRC_LISTEN_ADDR` | `0.0.0.0` | ❌ | Interface address for incoming traffic |
| `WEBHOOK_LISTEN_ADDR` | `0.0.0.0` | ❌ | Interface address for HTTP endpoints |
| `WEBHOOK_LISTEN_PORT` | `8081` | ❌ | Port for HTTP endpoints |

> [!IMPORTANT]
> When configuring multiple ports, the order of `SRC_PORTS` must match `DEST_PORTS`.
> Example: To forward 8080→80 and 7070→443, use:
>
> - `SRC_PORTS=8080,7070`
> - `DEST_PORTS=80,443`

### Target IPv6 Address

The target IPv6 address is stored in `data/ipv6_address.txt` and can be updated with a HTTP webhook:

```bash
curl 'http://localhost:8081/update' \
  -H 'Authorization: Bearer your-token-here' \
  -d 'IPv6: 2001:db8::1'
```

> [!NOTE]  
> Four2Six is currently only writing to the file, not reading. So updating it manually has no effect.

Originally, i wanted to expect a proper formatted JSON payload but since cloudflare-ddns just sends some text without formatting etc, i've decided to ~~steal~~ add a regex expression that just parses the received text for an IPv6 address.

### Health Check Endpoint

Monitor tunnel health status:

```bash
curl 'http://localhost:8081/health'
```

Response example:

```json
[
  {
    "ipv4_port": "80",
    "ipv6_port": "80",
    "ipv6_alive": true
  }
]
```

> If any target port is not reachable, the `/health` endpoint will respond with HTTP 500.

## 🐳 Docker Deployment

The preferred way to run Four2Six is by using Docker. You can always compile the [main.go](main.go) yourself and run it as a binary directly of course.

### Docker and IPv6

In order to connect to a IPv6 address within a docker container, connect it to a IPv6 capable docker network. Make sure that your Docker host system is able to connect to IPv6 networks. Read more about it [here](https://docs.docker.com/engine/daemon/ipv6/).

### Quick Start

Create an IPv6 capable network:

```bash
docker network create --ipv6 --subnet 2001:db8::/64 ip6net
```

Start the container with minimal config:

```bash
docker run -d \
  --name four2six \
  --network ip6net \
  -v "${PWD}/data:/app/data" \
  -p 8080:8080 \
  -p 8081:8081 \
  -e WEBHOOK_TOKEN=your-token-here \
  ghcr.io/muckelba/four2six:main
```

### Docker Compose

```yaml
services:
  four2six:
    image: ghcr.io/muckelba/four2six:main
    restart: unless-stopped
    networks:
      - ipv6net
    env_file: .env
    volumes:
      - ./data:/app/data
    ports:
      - "8080:8080"
      - "8081:8081"

networks:
  ipv6net:
    enable_ipv6: true
    ipam:
      config:
        - subnet: 2001:db8::/64
```

## 🔧 Advanced Setup

### Traefik Integration

Example configuration for running behind Traefik proxy:

1. Create `.env`:

```ini
WEBHOOK_TOKEN=your-secure-token
SRC_PORTS=80,443
DEST_PORTS=80,443
```

1. Configure `compose.yaml`:

```yaml
services:
  four2six:
    image: ghcr.io/muckelba/four2six:main
    restart: unless-stopped
    networks:
      - default
      - traefik
    env_file: .env
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

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
