/**
 * Polls the backend's /api/health/detailed endpoint and exposes
 * reactive service-level health status.
 *
 * Overall: "green" = all ok, "yellow" = optional services (TTS/STT) down,
 * "red" = critical service (agent) down.
 */

interface ServiceStatus {
  name: string;
  available: boolean;
  error?: string;
}

interface DetailedHealth {
  overall: 'green' | 'yellow' | 'red';
  services: ServiceStatus[];
}

const POLL_INTERVAL_MS = 10_000; // 10 seconds

// Module-level state so all callers share the same poll
let pollTimer: ReturnType<typeof setInterval> | null = null;
let subscriberCount = 0;

const health = ref<DetailedHealth>({
  overall: 'red',
  services: [],
});

const loading = ref(false);

async function fetchHealth() {
  const base = resolveBackendUrl();
  try {
    loading.value = true;
    const data = await $fetch<DetailedHealth>(`${base}/api/health/detailed`, {
      timeout: 5000,
    });
    health.value = data;
  } catch {
    // Backend unreachable — mark everything as red
    health.value = {
      overall: 'red',
      services: [
        { name: 'backend', available: false, error: 'unreachable' },
      ],
    };
  } finally {
    loading.value = false;
  }
}

function startPolling() {
  if (pollTimer) return;
  // Fetch immediately, then every POLL_INTERVAL_MS
  fetchHealth();
  pollTimer = setInterval(fetchHealth, POLL_INTERVAL_MS);
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

export function useHealth() {
  // Track subscribers so we only poll while at least one component uses it
  subscriberCount++;
  startPolling();

  onUnmounted(() => {
    subscriberCount--;
    if (subscriberCount <= 0) {
      subscriberCount = 0;
      stopPolling();
    }
  });

  return {
    health: readonly(health),
    loading: readonly(loading),
    refresh: fetchHealth,
  };
}
