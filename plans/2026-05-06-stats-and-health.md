# Plan: System Stats & Health Overview

**Status:** draft
**Created:** 2026-05-06
**Author:** jenpet + planitect

## Goal

Provide at-a-glance system health and detailed stats for all running Voilot services and agent instances, accessible from the existing header status dot without leaving the current page.

## Context

- The header already has a status dot (StatusIndicator.vue) that opens a dropdown showing agent availability (with instance count), TTS, and STT status.
- The backend exposes `GET /api/health/detailed` returning overall status and per-service health.
- The ProviderRegistry already tracks all running instances with worktree, provider name, PID, base URL, last activity, and idle/active state.
- Multiple providers (OpenCode, Pi, etc.) are supported via the multi-provider registry.
- Currently there is no per-instance breakdown or spawn timestamps exposed to the frontend.

## Approach

### 1. Backend: Extend `/api/health/detailed`

- Add `SpawnedAt time.Time` field to `agent.Instance` (set in `GetOrSpawn`).
- Extend the `/api/health/detailed` response to include a per-instance list alongside the existing service-level summary:
  ```json
  {
    "overall": "green",
    "services": [
      { "name": "opencode", "available": true, "instances": 2 },
      { "name": "pi", "available": true, "instances": 1 },
      { "name": "tts", "available": true },
      { "name": "stt", "available": true }
    ],
    "instances": [
      {
        "worktree": "/path/to/project",
        "provider": "opencode",
        "pid": 12345,
        "port": 4096,
        "baseUrl": "http://localhost:4096",
        "active": false,
        "spawnedAt": "2026-05-05T10:00:00Z",
        "lastActivity": "2026-05-05T10:05:00Z"
      }
    ]
  }
  ```
- Change the agent service rows from a single "agent" entry to one row per registered provider, each with its own availability and instance count.
- Derive port from the instance's `BaseURL` where possible.
- Same endpoint serves both the dropdown summary and the detail modal — the frontend decides what to render.

### 2. Frontend: Enhance StatusIndicator dropdown

- Show one row per provider (instead of a single "agent" row) with instance count and active/idle breakdown (e.g., "2 instances, 1 active").
- Keep TTS/STT rows as-is (ok/down).
- Add a "View details" link at the bottom that opens the detail modal.

### 3. Frontend: Reusable fullscreen overlay component

- Create a reusable `FullscreenOverlay.vue` component:
  - Dark semi-transparent backdrop.
  - Padded content area with rounded corners.
  - Slot-based content injection.
  - Close on backdrop click and escape key.
  - Smooth enter/leave transitions.
- This component will be reused for other overlays (e.g., PWA install prompt).

### 4. Frontend: Status detail modal

- Triggered from the dropdown "View details" link; opens in the FullscreenOverlay.
- **Per-instance table/list**: worktree (short path), provider name, PID, port, active/idle badge, spawned-at timestamp, last activity timestamp.
- **Service health section**: TTS and STT with configured URLs.
- **Polling**: refresh data from `/api/health/detailed` every 5 seconds while the modal is open. Provides near-real-time visibility into instance lifecycle (spawning, reaping, eviction) without WebSocket complexity.

## Open Questions

- Should the polling interval be configurable, or is 5 seconds a sensible fixed default?
- Should the detail modal show historical events (instance killed/spawned), or only current state? Current plan is current state only.

## Acceptance Criteria

- [ ] `Instance` struct has `SpawnedAt` field, set on spawn.
- [ ] `/api/health/detailed` returns per-provider service rows and a per-instance list with worktree, provider, PID, port, active/idle, spawnedAt, lastActivity.
- [ ] StatusIndicator dropdown shows one row per provider with instance count and active/idle summary.
- [ ] Dropdown includes a "View details" link opening the fullscreen modal.
- [ ] `FullscreenOverlay.vue` is a reusable component with backdrop, padding, slot, close-on-click/escape.
- [ ] Detail modal displays per-instance info and service health, polling every few seconds.
- [ ] Modal closes cleanly, polling stops, user returns to previous context.
