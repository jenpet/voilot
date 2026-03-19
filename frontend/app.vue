<template>
  <div class="min-h-screen bg-surface-900 text-surface-100">
    <NuxtPage />
  </div>
</template>

<script setup lang="ts">
// Unregister any stale PWA service workers (PWA module disabled during dev)
if (import.meta.client && 'serviceWorker' in navigator) {
  navigator.serviceWorker.getRegistrations().then((registrations) => {
    for (const reg of registrations) {
      reg.unregister();
      console.log('[voilot] Unregistered stale service worker:', reg.scope);
    }
  });
}
</script>
