# Reverse proxy

Pocket Garage serves the UI, the API and the LubeLogger-compatible API from one port. Terminate TLS in front of it and set `BASE_URL` and `SECURE_COOKIES=true`.

## Caddy

```
garage.example.com {
	reverse_proxy pocket-garage:8080
}
```

## Traefik labels

```yaml
labels:
  traefik.enable: "true"
  traefik.http.routers.garage.rule: Host(`garage.example.com`)
  traefik.http.routers.garage.entrypoints: websecure
  traefik.http.routers.garage.tls.certresolver: le
  traefik.http.services.garage.loadbalancer.server.port: "8080"
```

## nginx

```
server {
  server_name garage.example.com;
  location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
    client_max_body_size 60m;
  }
}
```

## Forward auth

If you put Authelia / Authentik / Tinyauth in front and use `AUTH_MODE=proxy`, exclude these paths from the auth rule — their clients send their own credentials:

- `/api/*` — the LubeLogger-compatible API (phone app, Raycast) uses Basic auth or a bearer token
- `/cal/*` — calendar subscriptions carry `?token=`
- `/healthz`, `/metrics`

## A one-tap fuel page

The route `/quick?kind=fuel` is a single-screen fill-up form that remembers the vehicle. Give it its own hostname if you like a PWA that opens straight on it:

```
fuel.example.com {
	redir / /quick?kind=fuel 302
	reverse_proxy pocket-garage:8080
}
```
