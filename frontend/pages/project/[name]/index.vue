<template>
  <div class="flex flex-col h-screen">
    <!-- Header -->
    <header class="flex items-center justify-between px-4 py-3 bg-surface-800 border-b border-surface-700">
      <div class="flex items-center gap-2">
        <button
          class="text-surface-400 hover:text-surface-200 transition-colors"
          @click="router.push('/')"
        >
          &larr;
        </button>
        <h1 class="text-lg font-semibold">{{ projectName }}</h1>
      </div>
      <button
        class="px-3 py-1.5 text-sm rounded-lg bg-surface-700 hover:bg-surface-600 transition-colors"
        @click="showNewWorktree = true"
      >
        + Worktree
      </button>
    </header>

    <!-- New worktree input -->
    <div v-if="showNewWorktree" class="px-4 py-3 bg-surface-800 border-b border-surface-700">
      <form class="flex gap-2" @submit.prevent="doCreateWorktree">
        <input
          v-model="newDescription"
          type="text"
          placeholder="Short description (e.g. PWA offline support)"
          class="flex-1 px-3 py-1.5 text-sm rounded-lg bg-surface-900 border border-surface-600 text-surface-100 placeholder:text-surface-500 focus:outline-none focus:border-blue-500"
          autofocus
        />
        <button
          type="submit"
          class="px-3 py-1.5 text-sm rounded-lg bg-blue-600 hover:bg-blue-500 text-white transition-colors"
          :disabled="!newDescription.trim()"
        >
          Create
        </button>
        <button
          type="button"
          class="px-3 py-1.5 text-sm rounded-lg bg-surface-700 hover:bg-surface-600 transition-colors"
          @click="showNewWorktree = false; newDescription = ''"
        >
          Cancel
        </button>
      </form>
    </div>

    <!-- Worktree List -->
    <main class="flex-1 overflow-y-auto p-4">
      <div v-if="!project" class="flex items-center justify-center h-full text-surface-400">
        <p>Project not found</p>
      </div>

      <div v-else class="space-y-3 max-w-2xl mx-auto">
        <div
          v-for="wt in project.worktrees"
          :key="wt.name"
          class="bg-surface-800 border border-surface-700 rounded-xl p-4 cursor-pointer hover:bg-surface-750 transition-colors"
          @click="navigateToWorktree(wt)"
        >
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <h2 class="text-base font-medium">{{ wt.name }}</h2>
              <span
                v-if="wt.isMain"
                class="px-1.5 py-0.5 text-xs rounded bg-blue-600/20 text-blue-300"
              >
                main
              </span>
            </div>
            <div v-if="!wt.isMain" class="flex items-center gap-1">
              <button
                v-if="confirmingDelete !== wt.name"
                class="p-1.5 rounded-lg hover:bg-surface-600 text-surface-400 hover:text-red-400 transition-colors"
                @click.stop="confirmingDelete = wt.name"
              >
                &times;
              </button>
              <template v-else>
                <button
                  class="px-2 py-1 text-xs rounded-lg bg-red-600 hover:bg-red-500 text-white transition-colors"
                  @click.stop="doRemoveWorktree(wt.name)"
                >
                  Remove
                </button>
                <button
                  class="px-2 py-1 text-xs rounded-lg bg-surface-700 hover:bg-surface-600 transition-colors"
                  @click.stop="confirmingDelete = ''"
                >
                  Cancel
                </button>
              </template>
            </div>
          </div>
          <div class="mt-1 text-sm text-surface-400 font-mono">
            {{ wt.branch }}
          </div>
        </div>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import type { Worktree } from '~/composables/useWorkspace';

const router = useRouter();
const route = useRoute();
const { projects, createWorktree, removeWorktree } = useWorkspace();

const projectName = computed(() => route.params.name as string);
const project = computed(() => projects.value.find(p => p.name === projectName.value) || null);

const showNewWorktree = ref(false);
const newDescription = ref('');
const confirmingDelete = ref('');

async function doCreateWorktree() {
  const desc = newDescription.value.trim();
  if (!desc) return;
  await createWorktree(projectName.value, desc);
  newDescription.value = '';
  showNewWorktree.value = false;
}

function navigateToWorktree(wt: Worktree) {
  router.push(`/project/${projectName.value}/worktree/${encodeURIComponent(wt.name)}`);
}

async function doRemoveWorktree(worktreeName: string) {
  await removeWorktree(projectName.value, worktreeName);
  confirmingDelete.value = '';
}
</script>
