# Plan: Voilot Private Production via Tailscale + Pull-Based Updates

**Status:** draft
**Created:** 2026-04-04
**Author:** user + planitect

## Goal
Deploy Voilot in a production-like setup that is private by default and reachable only by your trusted devices, using Tailscale for network access control. Use host-initiated pull-based release updates so deployment does not require storing server SSH credentials in the repository.

## Context
- Current Voilot stack is Docker Compose based and already includes frontend, backend, and optional STT/TTS services.
- Current HTTPS setup is useful for local/mobile trust, but access control should move to network-level private access first.
- Requirement is single-user or very small trusted-user access ("only I can authenticate").
- Requirement is also to avoid repo-hosted SSH deployment credentials.
- Target environments: either VPS edge host or home server.

## Approach
1. **Adopt a private-by-default network model (Tailscale-first)**
   - Install Tailscale on host and client devices.
   - Disable public exposure of Voilot ports (3000/3443) at cloud firewall/router + host firewall.
   - Access Voilot only via Tailnet IP or MagicDNS.
   - Define ACL policy so only your user/devices can reach the Voilot node and service ports.

2. **Standardize runtime topology for both VPS and home deployment**
   - Run Voilot as Docker Compose on Linux host.
   - Keep OpenCode placement explicit:
     - Option A: OpenCode runs on same host.
     - Option B: OpenCode runs on another Tailscale node and is reached over Tailnet.
   - Define one canonical `.env` template for all required variables.

3. **Automate base machine bootstrap (lightweight)**
   - Use **cloud-init** (or equivalent first-boot script) for host setup:
     - Create non-root admin user
     - Install Docker + Compose plugin
     - Install Tailscale
     - Configure system updates + basic firewall defaults
     - Pull Voilot repo to target path
   - Keep this minimal in v1; add heavier configuration automation only if operational pain appears.

4. **Use pull-based release deployment from the host**
   - CI publishes versioned container artifacts.
   - Host polls release channel on a schedule.
   - Host decides and applies updates locally (`pull` + `up -d`).
   - No CI-to-host SSH login path required.
   - Include health verification and fallback to previous known-good version on failure.

5. **Harden access and transport**
   - Primary gate: Tailscale ACLs/tags and device auth.
   - Keep nginx TLS enabled for browser/mic compatibility, but treat Tailnet as the exposure boundary.
   - Remove any accidental `0.0.0.0` public ingress at infrastructure level.
   - Optionally add second factor later:
     - HTTP auth proxy (e.g., Authelia/Authentik/Cloudflare Access)
     - mTLS client certs for stricter device identity

6. **Define observability and operability baseline**
   - Health verification: `/api/health` and `/api/health/detailed`.
   - Log collection baseline via `docker compose logs` and restart policies.
   - Backup plan:
     - Repo + deployment config
     - Any persistent STT cache volumes if desired
   - Recovery test: recreate host from automation and restore service in one runbook.

7. **Create rollout strategy**
   - Phase 1: home server private rollout (manual update confirmation).
   - Phase 2: VPS private rollout via same model.
   - Phase 3: automatic updates with health gate + rollback.
   - Phase 4: optional public entry point only if needed, with explicit auth gateway.

## Open Questions
1. Should host updates auto-apply immediately, or only after manual confirmation in v1?
2. What release channel should the host follow (stable only vs all tags)?
3. Where will OpenCode run in production (same node as Voilot or separate Tailnet node)?
4. Do you want voice services (STT/TTS) enabled in production from day one, or text-only first?
5. Should we add a second auth layer now (basic auth proxy or mTLS), or defer until after Tailscale rollout?

## Acceptance Criteria
- Voilot is reachable only over Tailscale from your approved devices.
- Public internet cannot directly access Voilot service ports.
- A fresh machine can be bootstrapped automatically and run Voilot without manual snowflake steps.
- Deployment is reproducible on both home and VPS targets using the same automation approach.
- Host can detect and apply newer approved Voilot versions without CI SSH deployment.
- Failed updates can fall back to a known-good version.
- Health endpoints report green/yellow as expected and basic recovery procedure is documented and tested.
