# Tailscale CLI (Homebrew formula) over macOS GUI app

The Tailscale macOS GUI app installs a Network Extension that intercepts all traffic to/from known tailnet peers at the kernel level. This breaks LAN connectivity between machines that are both members of the same tailnet — even when Tailscale is "off," the extension remains active and silently drops or redirects peer traffic. The extension state persists across app restarts and can only be fully removed via Recovery Mode (SIP must be temporarily disabled).

We use the Homebrew CLI formula (`brew install tailscale`) instead. `tailscaled` runs as a LaunchDaemon with a standard utun interface — no Network Extension, no packet filter insertion, no LAN interference. `tailscale serve` provides HTTPS termination for the frontend (`:443` → `:3000`) and backend (`:8080` → `:8080`), which is the only Tailscale feature voilot requires.

## Considered Options

- **Tailscale macOS app (GUI/cask)** — provides a menu bar UI and auto-updates, but installs a Network Extension that claims ownership of LAN traffic between tailnet peers. This caused complete LAN isolation (no ping, no SSH) between co-located machines and required Recovery Mode + SIP disable to fully remove.
- **Tailscale CLI with `--tun=userspace-networking`** — avoids the tun interface entirely, but `tailscale serve` cannot accept inbound connections in this mode (no listener is created). Ruled out.
- **Tailscale CLI with real tun (chosen)** — creates a standard utun interface for inbound `tailscale serve` traffic. No Network Extension means no LAN interference. `--accept-routes=false` prevents route injection.

## Consequences

- No GUI menu bar icon — Tailscale status is checked via `tailscale status` CLI or `task deploy:status`.
- `tailscaled` runs as a system LaunchDaemon (`/Library/LaunchDaemons/`), state stored at `/var/lib/tailscale`.
- Initial authentication requires `tailscale up` with manual browser-based login (one-time during `task deploy:setup`).
- The `--shields-up` flag must NOT be used — it blocks all inbound connections including `tailscale serve` traffic from phones.
