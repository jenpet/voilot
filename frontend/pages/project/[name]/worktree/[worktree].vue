<template>
  <div class="flex flex-col h-screen safe-top">
    <!-- Header -->
    <header class="bg-bg-secondary border-b border-bg-elevated">
      <div class="flex items-center justify-between px-4 py-5 max-w-[1200px] mx-auto">
      <div class="flex items-center gap-2">
        <button
          class="text-text-muted hover:text-text-primary transition-colors"
          @click="router.push(`/project/${projectName}`)"
        >
          &larr;
        </button>
        <div>
          <h1 class="text-lg font-semibold">{{ worktreeName }}</h1>
          <p class="text-xs text-text-muted">{{ projectName }}</p>
        </div>
      </div>
      <div class="flex items-center gap-1">
        <select
          v-if="availableProviders.length >= 1"
          v-model="selectedProvider"
          class="text-sm bg-bg-secondary border border-bg-elevated rounded-lg px-2 py-1.5 text-text-primary outline-none"
        >
          <option v-for="p in availableProviders" :key="p" :value="p">{{ p }}</option>
        </select>
        <button
          class="px-3 py-1.5 text-sm rounded-lg bg-bg-secondary hover:bg-bg-elevated transition-colors flex items-center gap-1.5"
          :disabled="creating"
          :class="{ 'opacity-50 pointer-events-none': creating }"
          @click="createSessionForWorktree"
        >
          <SpinnerIcon v-if="creating" class="w-4 h-4" />
          <template v-else>+ New Session</template>
        </button>
      </div>
      </div>
    </header>

    <!-- Session List -->
    <main class="flex-1 overflow-y-auto p-4">
      <!-- Loading state -->
      <div v-if="loading" class="flex items-center justify-center h-full">
        <LoadingLogo size="30%" message="Loading sessions..." />
      </div>

      <!-- Error state -->
      <div v-else-if="loadError" class="flex flex-col items-center justify-center h-full text-center">
        <p class="text-accent-warn mb-2">Failed to load sessions</p>
        <button
          class="text-sm text-text-muted hover:text-text-primary underline transition-colors"
          @click="loadWorktreeSessions"
        >
          Retry
        </button>
      </div>

      <!-- Empty state -->
      <div v-else-if="worktreeSessions.length === 0" class="flex flex-col items-center justify-center h-full text-text-muted">
        <p class="text-xl mb-2">No sessions yet</p>
        <p class="text-sm">Create a new session to start working in this worktree</p>
      </div>

      <!-- Session cards -->
      <div v-else class="space-y-3 max-w-[1200px] mx-auto">
        <SessionCard
          v-for="session in worktreeSessions"
          :key="session.id"
          :session="session"
          @click="router.push(`/session/${session.id}`)"
          @delete="deleteSession(session.id)"
        />
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
const router = useRouter();
const route = useRoute();
const { sessions, fetchSessions, clearSessions, createSession, deleteSession: doDeleteSession } = useSession();
const { projects } = useWorkspace();
const { providers, defaultProvider, getWorktreeDefault, setWorktreeDefault } = useProvider();

const projectName = computed(() => route.params.name as string);
const worktreeName = computed(() => decodeURIComponent(route.params.worktree as string));

const worktree = computed(() => {
  const project = projects.value.find(p => p.name === projectName.value);
  if (!project) return null;
  return project.worktrees.find(wt => wt.name === worktreeName.value) || null;
});

// Provider state
const availableProviders = computed(() => providers.value);
const selectedProvider = ref('');

// Initialize provider selection from worktree default (last used)
watch(worktree, async () => {
  if (!worktree.value) return;
  const wtDefault = await getWorktreeDefault(worktree.value.path);
  if (!selectedProvider.value) {
    selectedProvider.value = wtDefault || defaultProvider.value;
  }
}, { immediate: true });

// Local loading and error state (owned by this page, not the composable)
const loading = ref(false);
const loadError = ref(false);
const creating = ref(false);

// Sessions are fetched directly for the worktree via ?worktree= param
// Sort by last updated, most recent first
const worktreeSessions = computed(() =>
  [...sessions.value].sort((a, b) => (b.time?.updated ?? 0) - (a.time?.updated ?? 0))
);

async function loadWorktreeSessions() {
  if (!worktree.value) return;
  loading.value = true;
  loadError.value = false;
  try {
    await fetchSessions(worktree.value.path);
  } catch {
    loadError.value = true;
  } finally {
    loading.value = false;
  }
}

// Clear stale sessions and reload when worktree changes
watch(worktree, (newWt, oldWt) => {
  if (newWt?.path !== oldWt?.path) {
    clearSessions();
  }
  loadWorktreeSessions();
}, { immediate: true });

async function createSessionForWorktree() {
  if (!worktree.value || creating.value) return;
  creating.value = true;
  try {
    const session = await createSession({
      agent: 'planitect',
      provider: selectedProvider.value || undefined,
      worktreePath: worktree.value.path,
    });
    if (session) {
      // Persist selected provider as worktree default for next time
      if (selectedProvider.value) {
        setWorktreeDefault(worktree.value.path, selectedProvider.value);
      }
      // Refresh mapped sessions to include the new one
      await loadWorktreeSessions();
      router.push(`/session/${session.id}`);
    }
  } finally {
    creating.value = false;
  }
}

async function deleteSession(id: string) {
  await doDeleteSession(id);
  await loadWorktreeSessions();
}
</script>
