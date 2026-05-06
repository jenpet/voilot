<template>
  <div class="relative">
    <!-- Clickable status dot -->
    <button
      class="inline-flex items-center justify-center w-5 h-5 rounded-full focus:outline-none focus:ring-2 focus:ring-offset-1 focus:ring-offset-bg-primary focus:ring-accent/50"
      :title="tooltipText"
      @click="open = !open"
    >
      <span
        class="inline-block w-2.5 h-2.5 rounded-full transition-colors"
        :class="dotClass"
      />
    </button>

    <!-- Dropdown panel -->
    <Transition
      enter-active-class="transition ease-out duration-150"
      enter-from-class="opacity-0 translate-y-1"
      enter-to-class="opacity-100 translate-y-0"
      leave-active-class="transition ease-in duration-100"
      leave-from-class="opacity-100 translate-y-0"
      leave-to-class="opacity-0 translate-y-1"
    >
      <div
        v-if="open"
        class="absolute left-0 top-full mt-2 w-56 rounded-lg bg-bg-secondary border border-bg-elevated shadow-lg z-50 py-2 px-3"
      >
        <div class="text-xs font-medium text-text-primary mb-2 uppercase tracking-wide">
          Service Status
        </div>
        <ul class="space-y-1.5">
          <li
            v-for="svc in health.services"
            :key="svc.name"
            class="flex items-center justify-between text-sm"
          >
            <span class="text-text-primary capitalize">{{ svc.name }}</span>
            <span class="flex items-center gap-1.5">
              <span
                class="inline-block w-1.5 h-1.5 rounded-full"
                :class="svc.available ? 'bg-accent' : 'bg-accent-warn'"
              />
              <span
                class="text-xs"
                :class="svc.available ? 'text-accent' : 'text-accent-warn'"
              >
                {{ statusLabel(svc) }}
              </span>
            </span>
          </li>
        </ul>
        <button
          class="mt-3 w-full text-xs text-text-secondary hover:text-text-primary text-center py-1 border-t border-bg-elevated"
          @click="showDetail = true; open = false"
        >
          View details
        </button>
      </div>
    </Transition>

    <!-- Click-away overlay (invisible) -->
    <div
      v-if="open"
      class="fixed inset-0 z-40"
      @click="open = false"
    />

    <!-- Status detail modal -->
    <FullscreenOverlay v-model="showDetail">
      <StatusDetailPanel />
    </FullscreenOverlay>
  </div>
</template>

<script setup lang="ts">
const { health } = useHealth();
const open = ref(false);
const showDetail = ref(false);

function statusLabel(svc: { available: boolean; error?: string; instances?: number; active?: number }) {
  if (!svc.available) return svc.error || 'down';
  if (svc.instances != null) {
    const n = svc.instances;
    const a = svc.active ?? 0;
    if (n === 0) return 'ready';
    return a > 0 ? `${n} (${a} active)` : `${n} idle`;
  }
  return 'ok';
}

const dotClass = computed(() => {
  switch (health.value.overall) {
    case 'green':
      return 'bg-accent';
    case 'yellow':
      return 'bg-accent-secondary animate-pulse';
    case 'red':
      return 'bg-accent-warn';
    default:
      return 'bg-accent-warn';
  }
});

const tooltipText = computed(() => {
  const down = health.value.services
    .filter(s => !s.available)
    .map(s => s.name);
  if (down.length === 0) return 'All services healthy';
  return `Down: ${down.join(', ')}`;
});
</script>
