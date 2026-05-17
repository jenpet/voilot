# Host-native backend and frontend, Docker for voice services only

The backend spawns OpenCode as a child process (`os/exec`) and shells out to `git` and `wt` for worktree management. Running the backend inside a Docker container would require installing OpenCode (Node.js + npm), Go toolchains, git, and worktrunk inside the image — turning a lean Alpine container into a heavy, fragile environment that duplicates what's already available on the host. The frontend has no independent reason to be containerized once the backend isn't.

We run the Go backend and Nuxt frontend natively on the host, managed by macOS launchd plists. Docker is used exclusively for TTS (Kokoro) and STT (faster-whisper), which are Python + ML model stacks that are significantly easier to manage as containers than as host-native installations (CUDA/PyTorch/model downloads).

## Considered Options

- **Everything in Docker** (original approach) — clean isolation, but impossible to spawn OpenCode child processes without bringing the entire Node.js/agent toolchain into the container. Also means maintaining heavy, multi-layer Dockerfiles for the backend.
- **Backend on host, frontend in Docker behind nginx** — still requires nginx for routing and static file serving. Unnecessary complexity when the frontend can run its own Node.js server and Tailscale handles TLS termination.
- **Host-native backend + frontend, Docker for voice** (chosen) — matches the local dev setup exactly. The backend spawns OpenCode naturally, git/wt are available natively, and deployment is a simple build + launchd restart.

## Consequences

- No nginx reverse proxy — the frontend and backend run on separate ports (3000, 8080). Tailscale exposes both over HTTPS. The browser makes cross-origin requests to the backend, handled by CORS.
- Deployment requires Go and Node.js installed on the host machine.
- `Dockerfile.backend` and `Dockerfile.frontend` are removed. The `docker/` directory is replaced by `deploy/docker/` containing only TTS/STT compose and Dockerfile.
