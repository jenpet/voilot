<template>
  <div class="flex flex-col h-screen safe-top">
    <!-- Header -->
    <header class="flex items-center justify-between px-4 py-5 bg-surface-800 border-b border-surface-700">
      <div class="flex items-center gap-2">
        <button
          class="text-surface-400 hover:text-surface-200 transition-colors"
          @click="router.push(`/project/${projectName}`)"
        >
          &larr;
        </button>
        <div>
          <h1 class="text-lg font-semibold">{{ worktreeName }}</h1>
          <p class="text-xs text-surface-400">{{ projectName }}</p>
        </div>
      </div>
      <button
        class="px-3 py-1.5 text-sm rounded-lg bg-surface-700 hover:bg-surface-600 transition-colors"
        @click="createSessionForWorktree"
      >
        + New Session
      </button>
    </header>

    <!-- Session List -->
    <main class="flex-1 overflow-y-auto p-4">
      <div v-if="worktreeSessions.length === 0" class="flex flex-col items-center justify-center h-full text-surface-400">
        <p class="text-xl mb-2">No sessions yet</p>
        <p class="text-sm">Create a new session to start working in this worktree</p>
      </div>

      <div v-else class="space-y-3 max-w-2xl mx-auto">
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
const { sessions, createSession, deleteSession: doDeleteSession } = useSession();
const { projects, fetchWorktreeSessions } = useWorkspace();

const projectName = computed(() => route.params.name as string);
const worktreeName = computed(() => decodeURIComponent(route.params.worktree as string));

const worktree = computed(() => {
  const project = projects.value.find(p => p.name === projectName.value);
  if (!project) return null;
  return project.worktrees.find(wt => wt.name === worktreeName.value) || null;
});

// Session IDs mapped to this worktree
const mappedSessionIds = ref<string[]>([]);

async function loadWorktreeSessions() {
  if (!worktree.value) return;
  mappedSessionIds.value = await fetchWorktreeSessions(worktree.value.path);
}

// Filter sessions to only those mapped to this worktree
const worktreeSessions = computed(() => {
  if (mappedSessionIds.value.length === 0) return [];
  const idSet = new Set(mappedSessionIds.value);
  return sessions.value.filter(s => idSet.has(s.id));
});

// Load on mount and when worktree changes
watch(worktree, () => loadWorktreeSessions(), { immediate: true });

async function createSessionForWorktree() {
  if (!worktree.value) return;
  const session = await createSession({
    mode: 'plan',
    agent: 'planitect',
    worktreePath: worktree.value.path,
  });
  if (session) {
    // Refresh mapped sessions to include the new one
    await loadWorktreeSessions();
    router.push(`/session/${session.id}`);
  }
}

async function deleteSession(id: string) {
  await doDeleteSession(id);
  await loadWorktreeSessions();
}
</script>
