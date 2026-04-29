<template>
  <div class="min-h-screen bg-bg-primary text-text-primary">
    <NuxtPage />
  </div>
</template>

<script setup lang="ts">
import { applyTheme } from '~/composables/useTheme';

onMounted(() => {
  applyTheme();
});
</script>
