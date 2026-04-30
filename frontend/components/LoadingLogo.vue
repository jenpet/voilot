<template>
  <div class="flex flex-col items-center justify-center" :style="{ gap: gap }">
    <img
      class="voilot-pulse"
      :style="{ width: size, height: size, maxWidth: maxWidth }"
      src="~/assets/svg/voilot-logo.svg"
      alt="voilot logo"
    >
    <p
      v-if="displayMessage"
      class="text-text-muted italic"
      :style="{ fontSize: fontSize }"
    >
      {{ displayMessage }}
    </p>
  </div>
</template>

<script setup lang="ts">
const props = withDefaults(defineProps<{
  /** Width/height of the logo image */
  size?: string;
  /** Max width of the logo image */
  maxWidth?: string;
  /** Font size for the subtitle text */
  fontSize?: string;
  /** Gap between logo and text */
  gap?: string;
  /** Custom message (overrides random pick). Pass empty string to hide. */
  message?: string;
  /** Whether to show the subtitle at all */
  showMessage?: boolean;
  /** Minimum display time in ms (prevents flash) */
  minDuration?: number;
}>(), {
  size: '50%',
  maxWidth: '200px',
  fontSize: '14px',
  gap: '16px',
  showMessage: true,
  minDuration: 1000,
});

// Minimum display timer — visible stays true for at least minDuration ms
const minActive = ref(true)
let timer: ReturnType<typeof setTimeout> | null = null
onMounted(() => {
  timer = setTimeout(() => { minActive.value = false }, props.minDuration)
})
onUnmounted(() => { if (timer) clearTimeout(timer) })

defineExpose({ minActive })

const MESSAGES = [
  'Clearing throat...',
  'Tuning the frequencies...',
  'Stretching vocal cords...',
  'Consulting the lobster oracle...',
  'Warming up the voice box...',
  'Summoning the muse...',
  'Calibrating sass levels...',
  'Inhaling deeply...',
  'Finding the right words...',
  'Adjusting the antenna...',
  'Brewing liquid confidence...',
  'Rehearsing in the mirror...',
];

const randomMessage = MESSAGES[Math.floor(Math.random() * MESSAGES.length)];
const displayMessage = computed(() => {
  if (!props.showMessage) return '';
  if (props.message !== undefined) return props.message;
  return randomMessage;
});
</script>

<style scoped>
.voilot-pulse {
  animation: voilot-pulse 2.5s ease-in-out infinite;
}

@keyframes voilot-pulse {
  0%, 100% {
    filter: brightness(0) saturate(100%) invert(72%) sepia(30%) saturate(600%) hue-rotate(142deg) brightness(95%) contrast(90%);
    transform: scale(0.95);
  }
  50% {
    filter: brightness(0) saturate(100%) invert(88%) sepia(30%) saturate(400%) hue-rotate(5deg) brightness(105%) contrast(90%);
    transform: scale(1.05);
  }
}
</style>
