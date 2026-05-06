<template>
  <div
    v-if="promptType"
    class="fixed inset-0 z-[100] bg-bg-primary flex flex-col items-center justify-center p-8"
  >
    <!-- App icon -->
    <img
      :src="iconSrc"
      :alt="appName"
      class="w-24 h-24 rounded-2xl mb-6 shadow-lg"
    />

    <!-- App name -->
    <h1 class="text-2xl font-bold text-text-primary mb-2">{{ appName }}</h1>
    <p class="text-text-muted text-sm mb-8">Voice-first AI agent</p>

    <!-- Install (Android) -->
    <template v-if="promptType === 'install-android'">
      <p class="text-text-primary text-center max-w-sm mb-8 leading-relaxed">
        Install {{ appName }} on your home screen for the best experience.
      </p>
      <button
        class="bg-accent hover:bg-accent active:bg-accent text-white rounded-xl px-8 py-4 text-lg font-medium transition-colors"
        @click="triggerInstall"
      >
        Install {{ appName }}
      </button>
    </template>

    <!-- Install (iOS) -->
    <template v-else-if="promptType === 'install-ios'">
      <div class="text-text-primary text-center max-w-sm mb-8 leading-relaxed space-y-4">
        <p>To install, follow these steps:</p>
        <div class="flex flex-col items-center gap-4 text-sm">
          <div class="flex items-center gap-2">
            <span class="text-text-primary font-medium bg-bg-secondary rounded-lg px-3 py-1.5">1</span>
            <span>Tap the</span>
            <!-- iOS Share icon (inline SVG) -->
            <svg class="w-5 h-5 text-accent inline-block" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M4 12v8a2 2 0 002 2h12a2 2 0 002-2v-8" />
              <polyline points="16 6 12 2 8 6" />
              <line x1="12" y1="2" x2="12" y2="15" />
            </svg>
            <span>Share button</span>
          </div>
          <div class="flex items-center gap-2">
            <span class="text-text-primary font-medium bg-bg-secondary rounded-lg px-3 py-1.5">2</span>
            <span>Scroll down and tap</span>
          </div>
          <div class="flex items-center gap-2 text-text-primary font-medium bg-bg-secondary rounded-lg px-4 py-2">
            <!-- Plus icon -->
            <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round">
              <line x1="12" y1="5" x2="12" y2="19" />
              <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            <span>Add to Home Screen</span>
          </div>
        </div>
      </div>
      <button
        class="bg-accent hover:bg-accent active:bg-accent text-white rounded-xl px-8 py-4 text-lg font-medium transition-colors"
        @click="dismiss"
      >
        Got it
      </button>
    </template>

    <!-- Update available -->
    <template v-else-if="promptType === 'update'">
      <p class="text-text-primary text-center max-w-sm mb-8 leading-relaxed">
        A new version of {{ appName }} is available.
      </p>
      <button
        class="bg-accent hover:bg-accent active:bg-accent text-white rounded-xl px-8 py-4 text-lg font-medium transition-colors"
        @click="triggerUpdate"
      >
        Update now
      </button>
    </template>

    <!-- Dismiss link -->
    <button
      v-if="promptType !== 'install-ios'"
      class="text-text-muted hover:text-text-primary text-sm mt-6 transition-colors"
      @click="dismiss"
    >
      {{ promptType === 'update' ? 'Later' : 'Not now' }}
    </button>
  </div>
</template>

<script setup lang="ts">
const { promptType, triggerInstall, triggerUpdate, dismiss } = usePwaPrompt();

const isDev = import.meta.dev;
const appName = isDev ? 'voilot dev' : 'voilot';
const iconSrc = isDev ? '/icons/dev/icon-192x192.png' : '/icons/icon-192x192.png';
</script>
