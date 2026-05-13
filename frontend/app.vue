<template>
  <div class="min-h-screen bg-bg-primary text-text-primary">
    <NuxtPage />
  </div>
</template>

<script setup lang="ts">
import { applyTheme } from '~/composables/useTheme';
import { unlockAudio } from '~/composables/useTTS';

onMounted(() => {
  applyTheme();

  // Unlock iOS Safari AudioContext on first user interaction.
  // This one-shot handler ensures all downstream audio (TTS, feedback
  // tones, blips) can play without requiring per-call user gestures.
  const unlock = () => {
    unlockAudio();
    document.removeEventListener('click', unlock);
    document.removeEventListener('touchstart', unlock);
  };
  document.addEventListener('click', unlock, { once: true });
  document.addEventListener('touchstart', unlock, { once: true });
});
</script>
