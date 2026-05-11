<template>
   <div class="flex flex-col h-screen safe-top">
    <!-- Header -->
    <header class="bg-bg-secondary border-b border-bg-elevated">
      <div class="flex items-center justify-between px-4 py-3 max-w-[1200px] mx-auto">
      <div class="flex items-center gap-2">
        <StatusIndicator />
      </div>
      <div class="flex items-center gap-1.5">
        <img
          class="h-10 brightness-0 invert"
          src="~/assets/svg/voilot-logo.svg"
          alt="voilot"
        >
        <span class="text-lg font-semibold">voilot</span>
      </div>
      <div class="flex items-center gap-2">
        <button
          v-if="canShowInstall && !promptType"
          class="px-3 py-1.5 text-sm rounded-lg bg-accent/20 text-accent hover:bg-accent/30 transition-colors"
          @click="reopen"
        >
          Install
        </button>
        <button
          class="px-3 py-1.5 text-sm rounded-lg bg-bg-secondary hover:bg-bg-elevated transition-colors"
          @click="showAddProject = true"
        >
          + New
        </button>
      </div>
      </div>
    </header>

    <!-- Add project form -->
    <div v-if="showAddProject" class="bg-bg-secondary border-b border-bg-elevated">
      <div class="px-4 py-3 max-w-[1200px] mx-auto">
      <div class="flex gap-2 mb-3">
        <button
          class="px-3 py-1 text-sm rounded-lg transition-colors"
          :class="addMode === 'plain' ? 'bg-bg-elevated text-text-primary' : 'bg-bg-secondary text-text-muted hover:text-text-primary'"
          @click="addMode = 'plain'"
        >
          Plain
        </button>
        <button
          class="px-3 py-1 text-sm rounded-lg transition-colors"
          :class="addMode === 'import' ? 'bg-bg-elevated text-text-primary' : 'bg-bg-secondary text-text-muted hover:text-text-primary'"
          @click="addMode = 'import'"
        >
          Import
        </button>
        <button
          class="px-3 py-1 text-sm rounded-lg transition-colors"
          :class="addMode === 'clone' ? 'bg-bg-elevated text-text-primary' : 'bg-bg-secondary text-text-muted hover:text-text-primary'"
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
          class="flex-1 px-3 py-1.5 text-sm rounded-lg bg-bg-primary border border-bg-elevated text-text-primary placeholder:text-text-muted focus:outline-none focus:border-accent disabled:opacity-50"
          :disabled="addLoading"
          autofocus
        />
          <button
            type="submit"
            class="px-3 py-1.5 text-sm rounded-lg bg-accent hover:bg-accent text-white transition-colors flex items-center gap-1.5"
            :disabled="!projectInput.trim() || addLoading"
            :class="{ 'opacity-50 pointer-events-none': addLoading }"
          >
            <SpinnerIcon v-if="addLoading" class="w-4 h-4" />
            <template v-else>{{ addMode === 'import' ? 'Import' : addMode === 'clone' ? 'Clone' : 'Create' }}</template>
          </button>
        <button
          type="button"
          class="px-3 py-1.5 text-sm rounded-lg bg-bg-secondary hover:bg-bg-elevated transition-colors"
          :disabled="addLoading"
          :class="{ 'opacity-50 pointer-events-none': addLoading }"
          @click="showAddProject = false; projectInput = ''"
        >
          Cancel
        </button>
      </form>
      <p v-if="addError" class="mt-2 text-sm text-accent-warn">{{ addError }}</p>
      </div>
    </div>

    <!-- Project List -->
    <main class="flex-1 overflow-y-auto p-4">
      <div v-if="showLoading" class="flex items-center justify-center h-full">
        <LoadingLogo ref="workspaceLogoRef" message="Preparing workspace..." />
      </div>

      <div v-else-if="projects.length === 0" class="flex flex-col items-center justify-center h-full text-text-muted">
        <p class="text-xl mb-2">No projects found</p>
        <p class="text-sm">Configure --workspace-dir to discover projects</p>
      </div>

      <div v-else class="space-y-3 max-w-[1200px] mx-auto">
        <div
          v-for="project in projects"
          :key="project.name"
          class="bg-bg-secondary border border-bg-elevated rounded-xl p-4 cursor-pointer hover:bg-bg-elevated transition-colors"
          @click="navigateToProject(project.name)"
        >
          <div class="flex items-center justify-between">
            <h2 class="text-base font-medium">{{ project.name }}</h2>
            <span class="text-xs text-text-muted">
              {{ project.worktrees.length }} worktree{{ project.worktrees.length !== 1 ? 's' : '' }}
            </span>
          </div>
          <div class="mt-1 text-sm text-text-muted truncate">
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

// Min display time for workspace loading
const workspaceLogoRef = ref<InstanceType<typeof LoadingLogo>>()
const minLoadActive = ref(true)
watch(() => workspaceLogoRef.value?.minActive, (v) => {
  if (v === false) minLoadActive.value = false
})
const showLoading = computed(() => loading.value || minLoadActive.value)

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