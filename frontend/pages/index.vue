<template>
  <div class="flex flex-col h-screen">
    <!-- Header -->
    <header class="flex items-center justify-between px-4 py-3 bg-surface-800 border-b border-surface-700">
      <div class="flex items-center gap-2">
        <h1 class="text-lg font-semibold">voilot</h1>
        <span
          class="inline-block w-2 h-2 rounded-full"
          :class="{
            'bg-green-400': connectionState === 'connected',
            'bg-yellow-400 animate-pulse': connectionState === 'connecting',
            'bg-red-400': connectionState === 'disconnected',
          }"
          :title="`Backend: ${connectionState}`"
        />
      </div>
      <button
        class="px-3 py-1.5 text-sm rounded-lg bg-surface-700 hover:bg-surface-600 transition-colors"
        @click="createSession"
      >
        + New Session
      </button>
    </header>

    <!-- Session List -->
    <main class="flex-1 overflow-y-auto p-4">
      <div v-if="sessions.length === 0" class="flex flex-col items-center justify-center h-full text-surface-400">
        <p class="text-xl mb-2">No sessions yet</p>
        <p class="text-sm">Create a new session to start planning</p>
      </div>

      <div v-else class="space-y-3 max-w-2xl mx-auto">
        <SessionCard
          v-for="session in sessions"
          :key="session.id"
          :session="session"
          @click="navigateToSession(session.id)"
          @delete="deleteSession(session.id)"
        />
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
const router = useRouter()
const { sessions, createSession: doCreateSession, deleteSession: doDeleteSession } = useSession()
const { connectionState } = useWebSocket()

async function createSession() {
  const session = await doCreateSession({ mode: 'plan' })
  if (session) {
    router.push(`/session/${session.id}`)
  }
}

function navigateToSession(id: string) {
  router.push(`/session/${id}`)
}

async function deleteSession(id: string) {
  await doDeleteSession(id)
}
</script>
