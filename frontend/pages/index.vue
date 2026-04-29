<template>
   <div class="flex flex-col h-screen safe-top">
    <!-- Header -->
    <header class="flex items-center justify-between px-4 py-5 bg-surface-800 border-b border-surface-700">
      <div class="flex items-center gap-2">
        <h1 class="text-lg font-semibold">voilot</h1>
        <StatusIndicator />
      </div>
      <div class="flex items-center gap-2">
        <button
          v-if="canShowInstall && !promptType"
          class="px-3 py-1.5 text-sm rounded-lg bg-blue-600/30 text-blue-300 hover:bg-blue-600/50 transition-colors"
          @click="reopen"
        >
          Install
        </button>
        <button
          class="px-3 py-1.5 text-sm rounded-lg bg-surface-700 hover:bg-surface-600 transition-colors"
          @click="showAddProject = true"
        >
          + New
        </button>
      </div>
    </header>

    <!-- Add project form -->
    <div v-if="showAddProject" class="px-4 py-3 bg-surface-800 border-b border-surface-700">
      <div class="flex gap-2 mb-3">
        <button
          class="px-3 py-1 text-sm rounded-lg transition-colors"
          :class="addMode === 'plain' ? 'bg-surface-600 text-surface-100' : 'bg-surface-800 text-surface-400 hover:text-surface-200'"
          @click="addMode = 'plain'"
        >
          Plain
        </button>
        <button
          class="px-3 py-1 text-sm rounded-lg transition-colors"
          :class="addMode === 'import' ? 'bg-surface-600 text-surface-100' : 'bg-surface-800 text-surface-400 hover:text-surface-200'"
          @click="addMode = 'import'"
        >
          Import
        </button>
        <button
          class="px-3 py-1 text-sm rounded-lg transition-colors"
          :class="addMode === 'clone' ? 'bg-surface-600 text-surface-100' : 'bg-surface-800 text-surface-400 hover:text-surface-200'"
          @click="addMode = 'clone'"
        >
          Clone
        </button>
      </div>
      <form class="flex gap-2" @submit.prevent="doAddProject">
        <input
          v-model="projectInput"
          type="text"
          :placeholder="addMode === 'plain' ? 'Name for your scratch space (e.g. my-idea)' : addMode === 'import' ? 'Path to existing repo (e.g. ~/dev/myproject)' : 'Git URL (e.g. https://github.com/user/repo)'"
          class="flex-1 px-3 py-1.5 text-sm rounded-lg bg-surface-900 border border-surface-600 text-surface-100 placeholder:text-surface-500 focus:outline-none focus:border-blue-500"
          autofocus
        />
        <button
          type="submit"
          class="px-3 py-1.5 text-sm rounded-lg bg-blue-600 hover:bg-blue-500 text-white transition-colors"
          :disabled="!projectInput.trim() || addLoading"
        >
          {{ addLoading ? '...' : addMode === 'import' ? 'Import' : addMode === 'clone' ? 'Clone' : 'Create' }}
        </button>
        <button
          type="button"
          class="px-3 py-1.5 text-sm rounded-lg bg-surface-700 hover:bg-surface-600 transition-colors"
          @click="showAddProject = false; projectInput = ''"
        >
          Cancel
        </button>
      </form>
      <p v-if="addError" class="mt-2 text-sm text-red-400">{{ addError }}</p>
    </div>

    <!-- Project List -->
    <main class="flex-1 overflow-y-auto p-4">
      <div v-if="loading" class="flex items-center justify-center h-full text-surface-400">
        <p>Scanning workspace...</p>
      </div>

      <div v-else-if="projects.length === 0" class="flex flex-col items-center justify-center h-full text-surface-400">
        <p class="text-xl mb-2">No projects found</p>
        <p class="text-sm">Configure --workspace-dir to discover projects</p>
      </div>

      <div v-else class="space-y-3 max-w-2xl mx-auto">
        <div
          v-for="project in projects"
          :key="project.name"
          class="bg-surface-800 border border-surface-700 rounded-xl p-4 cursor-pointer hover:bg-surface-750 transition-colors"
          @click="navigateToProject(project.name)"
        >
          <div class="flex items-center justify-between">
            <h2 class="text-base font-medium">{{ project.name }}</h2>
            <span class="text-xs text-surface-400">
              {{ project.worktrees.length }} worktree{{ project.worktrees.length !== 1 ? 's' : '' }}
            </span>
          </div>
          <div class="mt-1 text-sm text-surface-400 truncate">
            {{ project.path }}
          </div>
        </div>
      </div>
    </main>

    <PwaPrompt />
  </div>
</template>

<script setup lang="ts">
const router = useRouter();
const { projects, loading, addProject, cloneProject, initProject } = useWorkspace();
const { promptType, canShowInstall, reopen } = usePwaPrompt();

const showAddProject = ref(false);
const projectInput = ref('');
const addError = ref('');
const addMode = ref<'plain' | 'import' | 'clone'>('plain');
const addLoading = ref(false);

function navigateToProject(name: string) {
  router.push(`/project/${name}`);
}

async function doAddProject() {
  const input = projectInput.value.trim();
  if (!input) return;
  addError.value = '';
  addLoading.value = true;
  try {
    const err = addMode.value === 'import'
      ? await addProject(input)
      : addMode.value === 'clone'
        ? await cloneProject(input)
        : await initProject(input);
    if (err) {
      addError.value = err;
    } else {
      projectInput.value = '';
      showAddProject.value = false;
    }
  } finally {
    addLoading.value = false;
  }
}
</script>
