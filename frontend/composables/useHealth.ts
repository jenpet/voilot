/**
 * Polls the backend's /api/health/detailed endpoint and exposes
 * reactive service-level health status and per-instance details.
 *
 * Overall: "green" = all ok, "yellow" = optional services (TTS/STT) down,
 * "red" = critical service (agent provider) down.
 */

interface ServiceStatus {
  name: string;
  available: boolean;
  error?: string;
  instances?: number;
  active?: number;
}

interface InstanceInfo {
  worktree: string;
  provider: string;
  pid: number;
  baseUrl: string;
  active: boolean;
  spawnedAt: string;
  lastActivity: string;
}

interface DetailedHealth {
  overall: 'green' | 'yellow' | 'red';
  services: ServiceStatus[];
  instances: InstanceInfo[];
}

const POLL_INTERVAL_MS = 10_000; // 10 seconds
const FAST_POLL_INTERVAL_MS = 5_000; // 5 seconds for detail view

// Module-level state so all callers share the same poll
let pollTimer: ReturnType<typeof setInterval> | null = null;
let subscriberCount = 0;
let currentInterval = POLL_INTERVAL_MS;

const health = ref<DetailedHealth>({
  overall: 'red',
  services: [],
  instances: [],
});

const loading = ref(false);

async function fetchHealth() {
  const base = resolveBackendUrl();
  try {
    loading.value = true;
    const data = await $fetch<DetailedHealth>(`${base}/api/health/detailed`, {
      timeout: 5000,
    });
    health.value = { ...data, instances: data.instances ?? [] };
  } catch {
    // Backend unreachable — mark everything as red
    health.value = {
      overall: 'red',
      services: [
        { name: 'backend', available: false, error: 'unreachable' },
      ],
      instances: [],
    };
  } finally {
    loading.value = false;
  }
}

function startPolling(interval = POLL_INTERVAL_MS) {
  if (pollTimer && currentInterval === interval) return;
  stopPolling();
  currentInterval = interval;
  fetchHealth();
  pollTimer = setInterval(fetchHealth, interval);
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

export function useHealth(opts?: { fastPoll?: boolean }) {
  const interval = opts?.fastPoll ? FAST_POLL_INTERVAL_MS : POLL_INTERVAL_MS;

  // Track subscribers so we only poll while at least one component uses it
  subscriberCount++;
  startPolling(interval);

  onUnmounted(() => {
    subscriberCount--;
    if (subscriberCount <= 0) {
      subscriberCount = 0;
      stopPolling();
    } else {
      // Revert to normal interval if no fast-poll subscribers remain
      startPolling(POLL_INTERVAL_MS);
    }
  });

  return {
    health: readonly(health),
    loading: readonly(loading),
    refresh: fetchHealth,
  };
}
