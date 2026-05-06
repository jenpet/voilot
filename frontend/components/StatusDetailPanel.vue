<template>
  <div>
    <h2 class="text-lg font-semibold text-text-primary mb-4">System Status</h2>

    <!-- Services overview -->
    <section class="mb-6">
      <h3 class="text-sm font-medium text-text-secondary uppercase tracking-wide mb-2">Services</h3>
      <div class="grid gap-2">
        <div
          v-for="svc in health.services"
          :key="svc.name"
          class="flex items-center justify-between p-3 rounded-lg bg-bg-secondary"
        >
          <div class="flex items-center gap-2">
            <span
              class="inline-block w-2 h-2 rounded-full"
              :class="svc.available ? 'bg-accent' : 'bg-accent-warn'"
            />
            <span class="text-text-primary capitalize font-medium">{{ svc.name }}</span>
          </div>
          <div class="text-sm text-text-secondary">
            <template v-if="!svc.available">
              <span class="text-accent-warn">{{ svc.error || 'down' }}</span>
            </template>
            <template v-else-if="svc.instances != null">
              {{ svc.instances }} instance{{ svc.instances !== 1 ? 's' : '' }}
              <span v-if="svc.active" class="text-accent">({{ svc.active }} active)</span>
            </template>
            <template v-else>
              <span class="text-accent">ok</span>
            </template>
          </div>
        </div>
      </div>
    </section>

    <!-- Instances detail -->
    <section>
      <h3 class="text-sm font-medium text-text-secondary uppercase tracking-wide mb-2">
        Agent Instances
        <span class="text-text-tertiary font-normal">({{ health.instances.length }})</span>
      </h3>

      <div v-if="health.instances.length === 0" class="text-sm text-text-secondary p-3 bg-bg-secondary rounded-lg">
        No running instances. Instances are spawned on demand.
      </div>

      <div v-else class="space-y-2">
        <div
          v-for="inst in health.instances"
          :key="`${inst.provider}-${inst.pid}`"
          class="p-3 rounded-lg bg-bg-secondary"
        >
          <div class="flex items-center justify-between mb-2">
            <div class="flex items-center gap-2">
              <span
                class="inline-block w-2 h-2 rounded-full"
                :class="inst.active ? 'bg-accent animate-pulse' : 'bg-text-tertiary'"
              />
              <span class="text-text-primary font-medium capitalize">{{ inst.provider }}</span>
              <span
                class="text-xs px-1.5 py-0.5 rounded"
                :class="inst.active ? 'bg-accent/20 text-accent' : 'bg-bg-elevated text-text-secondary'"
              >
                {{ inst.active ? 'active' : 'idle' }}
              </span>
            </div>
            <span class="text-xs text-text-tertiary font-mono">PID {{ inst.pid }}</span>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-1 text-xs text-text-secondary">
            <div class="truncate" :title="inst.worktree">
              <span class="text-text-tertiary">Path:</span> {{ inst.worktree }}
            </div>
            <div>
              <span class="text-text-tertiary">URL:</span> {{ inst.baseUrl }}
            </div>
            <div>
              <span class="text-text-tertiary">Spawned:</span> {{ formatTime(inst.spawnedAt) }}
            </div>
            <div>
              <span class="text-text-tertiary">Last active:</span> {{ formatTime(inst.lastActivity) }}
            </div>
          </div>
        </div>
      </div>
    </section>

    <!-- Polling indicator -->
    <div class="mt-4 text-xs text-text-tertiary text-center">
      Auto-refreshing every 5s
    </div>
  </div>
</template>

<script setup lang="ts">
const { health } = useHealth({ fastPoll: true });

function formatTime(iso: string): string {
  if (!iso) return '-';
  const d = new Date(iso);
  const now = Date.now();
  const diffMs = now - d.getTime();

  if (diffMs < 60_000) return 'just now';
  if (diffMs < 3_600_000) return `${Math.floor(diffMs / 60_000)}m ago`;
  if (diffMs < 86_400_000) return `${Math.floor(diffMs / 3_600_000)}h ago`;
  return d.toLocaleString();
}
</script>
