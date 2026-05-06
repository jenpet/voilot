# Troubleshooting

## Microphone doesn't work

**Cause:** `getUserMedia` requires a secure context (HTTPS or localhost).

- If accessing over the network, you must set up TLS. See [Deployment: TLS Termination](deployment.md#tls-termination).
- Tailscale is the easiest option for personal use: `tailscale serve --bg --https=443 http://localhost:3000`
- Localhost access works without HTTPS.

## Can't connect to OpenCode

**Symptoms:** Sessions fail to create, health check shows OpenCode as unreachable.

- Verify OpenCode is running: `curl http://localhost:4096/global/health`
- In Docker, the backend reaches the host via `host.docker.internal`. Ensure the `extra_hosts` mapping is present in `docker-compose.yml`.
- Check `config.docker.json` — the provider's binary path must be correct.
- On Linux, `host.docker.internal` may not resolve by default. Add `extra_hosts: ["host.docker.internal:host-gateway"]` to the backend service.

## TTS / STT not working

- Voice services require the `voice` profile: `docker compose --profile voice up`
- Check service health: `docker compose ps` should show `healthy` for tts and stt.
- Verify URLs in `config.docker.json` use Docker service names (`http://tts:8880`, `http://stt:5003`) not localhost.
- For local dev (non-Docker), use `http://localhost:8880` and `http://localhost:5003`.
- Check logs: `docker compose logs tts` or `docker compose logs stt`

## WebSocket disconnects

- Check nginx proxy timeout config. The default `proxy_read_timeout` for WebSocket should be long (e.g. `86400s`). This is already set in `voilot-common.conf`.
- If behind another reverse proxy (Caddy, Traefik, cloudflare), ensure it supports WebSocket upgrades and has appropriate timeouts.

## Container keeps restarting

Check logs for the specific service:

```bash
docker compose logs backend
docker compose logs frontend
docker compose logs tts
docker compose logs stt
```

Common causes:
- Backend: missing or invalid config file, OpenCode binary not found
- STT: model download failure (network issue), insufficient memory for larger models
- Frontend: nginx config syntax error

## Audio uploads fail (413 Request Entity Too Large)

nginx defaults to `1m` body size. voilot sets `client_max_body_size 25m` in `voilot-common.conf`. If you're behind another reverse proxy, ensure it also allows large uploads.
