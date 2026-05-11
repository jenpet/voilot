<template>
  <div class="flex flex-col h-screen safe-top">
    <!-- Header -->
    <header class="bg-bg-secondary border-b border-bg-elevated">
      <div class="flex items-center justify-between px-4 py-5 max-w-[1200px] mx-auto">
      <div class="flex items-center gap-2">
        <button
          class="text-text-muted hover:text-text-primary transition-colors"
          @click="router.push('/')"
        >
          &larr;
        </button>
        <h1 class="text-lg font-semibold">{{ projectName }}</h1>
      </div>
      <button
        class="px-3 py-1.5 text-sm rounded-lg bg-bg-secondary hover:bg-bg-elevated transition-colors"
        @click="showNewWorktree = true"
      >
        + Worktree
      </button>
      </div>
    </header>

    <!-- New worktree input -->
    <div v-if="showNewWorktree" class="bg-bg-secondary border-b border-bg-elevated">
      <div class="px-4 py-3 max-w-[1200px] mx-auto">
      <form class="flex flex-col gap-2" :class="{ 'opacity-50 pointer-events-none': creatingWorktree }" @submit.prevent="doCreateWorktree">
        <!-- Branch selector -->
        <div>
          <label class="block text-xs text-text-muted mb-1">Base branch</label>
          <select
            v-model="selectedBranch"
            class="w-full px-3 py-1.5 text-sm rounded-lg bg-bg-primary border border-bg-elevated text-text-primary focus:outline-none focus:border-accent"
            :disabled="branchesLoading || creatingWorktree"
          >
            <option v-if="branchesLoading" value="">Loading branches...</option>
            <option v-for="b in branches" :key="b.name" :value="b.name">
              {{ branchLabel(b) }}
            </option>
          </select>
        </div>
        <!-- Description -->
        <div class="flex gap-2">
          <input
            v-model="newDescription"
            type="text"
            placeholder="Short description (e.g. PWA offline support)"
            class="flex-1 px-3 py-1.5 text-sm rounded-lg bg-bg-primary border border-bg-elevated text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent"
          />
          <button
            type="submit"
            class="px-3 py-1.5 text-sm rounded-lg bg-accent hover:bg-accent text-white transition-colors flex items-center gap-1.5"
            :disabled="!newDescription.trim() || branchesLoading || creatingWorktree"
          >
            <SpinnerIcon v-if="creatingWorktree" class="w-4 h-4" />
            <template v-else>Create</template>
          </button>
          <button
            type="button"
            class="px-3 py-1.5 text-sm rounded-lg bg-bg-secondary hover:bg-bg-elevated transition-colors"
            :disabled="creatingWorktree"
            @click="showNewWorktree = false; newDescription = ''"
          >
            Cancel
          </button>
        </div>
      </form>
      <p v-if="createError" class="mt-2 text-sm text-accent-warn">{{ createError }}</p>
      </div>
    </div>

    <!-- Worktree List -->
    <main class="flex-1 overflow-y-auto p-4">
      <div v-if="!project" class="flex items-center justify-center h-full text-text-muted">
        <p>Project not found</p>
      </div>

      <div v-else class="space-y-3 max-w-[1200px] mx-auto">
        <ListCard
          v-for="wt in project.worktrees"
          :key="wt.name"
          :timestamp="wt.lastActivity"
          @click="navigateToWorktree(wt)"
        >
          <div class="flex items-center gap-2">
            <h2 class="text-base font-medium truncate">{{ wt.name }}</h2>
            <span
              v-if="wt.branch === project?.defaultBranch"
              class="px-1.5 py-0.5 text-xs rounded bg-accent/15 text-accent flex-shrink-0"
            >
              default
            </span>
          </div>
          <div class="mt-1 text-sm text-text-muted font-mono truncate">
            {{ wt.branch }}
          </div>
          <template #actions>
            <div class="flex items-center gap-1 flex-shrink-0">
              <template v-if="!wt.isRoot">
              <button
                v-if="confirmingDelete !== wt.name"
                class="p-1.5 rounded-lg hover:bg-bg-elevated text-text-muted hover:text-accent-warn transition-colors"
                @click.stop="confirmingDelete = wt.name"
              >
                &times;
              </button>
              <template v-else>
                <button
                  class="px-2 py-1 text-xs rounded-lg bg-accent-warn hover:bg-accent-warn text-white transition-colors flex items-center gap-1"
                  :disabled="removingWorktree === wt.name"
                  :class="{ 'opacity-50 pointer-events-none': removingWorktree === wt.name }"
                  @click.stop="doRemoveWorktree(wt.name)"
                >
                  <SpinnerIcon v-if="removingWorktree === wt.name" class="w-3 h-3" />
                  <template v-else>Remove</template>
                </button>
                <button
                  class="px-2 py-1 text-xs rounded-lg bg-bg-secondary hover:bg-bg-elevated transition-colors"
                  @click.stop="confirmingDelete = ''"
                >
                  Cancel
                </button>
              </template>
              </template>
            </div>
          </template>
        </ListCard>
      </div>
    </main>

    <!-- Remove error modal -->
    <FullscreenOverlay v-model="showRemoveError">
      <div class="flex flex-col min-h-full">
      <h2 class="text-lg font-semibold text-accent-warn mb-3">Failed to remove worktree</h2>
      <p class="text-sm text-text-primary">{{ removeError?.error }}</p>

      <div v-if="removeError?.files?.length" class="mt-4">
        <h3 class="text-xs font-medium text-text-muted uppercase mb-2">Affected files</h3>
        <ul class="space-y-1 text-sm font-mono">
          <li
            v-for="f in removeError.files"
            :key="f.path"
            class="flex items-center gap-2"
          >
            <span class="text-xs px-1.5 py-0.5 rounded bg-bg-elevated text-text-muted min-w-[80px] text-center flex-shrink-0">{{ f.status }}</span>
            <span class="text-text-primary break-all">{{ f.path }}</span>
          </li>
        </ul>
      </div>

      <div class="flex flex-col sm:flex-row gap-2 mt-auto pt-6">
        <button
          v-if="removeError?.forceable"
          class="w-full sm:w-auto px-3 py-2.5 sm:py-1.5 text-sm rounded-lg bg-accent-warn hover:bg-accent-warn/80 text-white transition-colors flex items-center justify-center gap-1.5"
          :disabled="forceRemoving"
          :class="{ 'opacity-50 pointer-events-none': forceRemoving }"
          @click="doForceRemove"
        >
          <SpinnerIcon v-if="forceRemoving" class="w-4 h-4" />
          <template v-else>Force Remove</template>
        </button>
        <button
          class="w-full sm:w-auto px-3 py-2.5 sm:py-1.5 text-sm rounded-lg bg-bg-elevated hover:bg-bg-secondary text-text-primary transition-colors"
          @click="showRemoveError = false"
        >
          Close
        </button>
      </div>
      </div>
    </FullscreenOverlay>
  </div>
</template>

<script setup lang="ts">
import type { Worktree, BranchInfo, RemoveError } from '~/composables/useWorkspace';

const router = useRouter();
const route = useRoute();
const { projects, createWorktree, removeWorktree, fetchBranches } = useWorkspace();

const projectName = computed(() => route.params.name as string);
const project = computed(() => projects.value.find(p => p.name === projectName.value) || null);

const showNewWorktree = ref(false);
const newDescription = ref('');
const selectedBranch = ref('');
const branches = ref<BranchInfo[]>([]);
const branchesLoading = ref(false);
const confirmingDelete = ref('');
const creatingWorktree = ref(false);
const removingWorktree = ref('');
const forceRemoving = ref(false);
const createError = ref('');
const removeError = ref<RemoveError | null>(null);
const failedWorktree = ref('');
const showRemoveError = computed({
  get: () => removeError.value !== null,
  set: (v: boolean) => { if (!v) removeError.value = null; },
});

const selectedBranchInfo = computed(() =>
  branches.value.find(b => b.name === selectedBranch.value) || null
);

function branchLabel(b: BranchInfo): string {
  const parts: string[] = [];
  if (b.hasRemote && b.behind > 0) parts.push(`\u2193${b.behind}`);
  if (b.hasRemote && b.ahead > 0) parts.push(`\u2191${b.ahead}`);
  const suffix = b.isDefault ? ' (default)' : !b.hasRemote ? ' (local)' : '';
  if (parts.length > 0) {
    return `${parts.join(' ')} ${b.name}${suffix}`;
  }
  return `${b.name}${suffix}`;
}

watch(showNewWorktree, async (visible) => {
  if (visible) {
    branchesLoading.value = true;
    branches.value = await fetchBranches(projectName.value);
    branchesLoading.value = false;
    // Default to the default branch, or first available
    const def = branches.value.find(b => b.isDefault);
    selectedBranch.value = def?.name || branches.value[0]?.name || '';
  }
});

async function doCreateWorktree() {
  const desc = newDescription.value.trim();
  if (!desc || creatingWorktree.value) return;
  createError.value = '';
  creatingWorktree.value = true;
  try {
    const base = selectedBranch.value || undefined;
    const { error } = await createWorktree(projectName.value, desc, base);
    if (error) {
      createError.value = error;
      return;
    }
    newDescription.value = '';
    showNewWorktree.value = false;
  } finally {
    creatingWorktree.value = false;
  }
}

function navigateToWorktree(wt: Worktree) {
  router.push(`/project/${projectName.value}/worktree/${encodeURIComponent(wt.name)}`);
}

async function doRemoveWorktree(worktreeName: string) {
  removingWorktree.value = worktreeName;
  try {
    const err = await removeWorktree(projectName.value, worktreeName);
    confirmingDelete.value = '';
    if (err) {
      failedWorktree.value = worktreeName;
      removeError.value = err;
    }
  } finally {
    removingWorktree.value = '';
  }
}

async function doForceRemove() {
  forceRemoving.value = true;
  try {
    const err = await removeWorktree(projectName.value, failedWorktree.value, true);
    if (err) {
      removeError.value = err;
    } else {
      removeError.value = null;
      failedWorktree.value = '';
    }
  } finally {
    forceRemoving.value = false;
  }
}
</script>