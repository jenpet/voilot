/**
 * Composable to fetch available providers and manage per-worktree default provider.
 */

interface ProvidersResponse {
  providers: string[];
  defaultProvider: string;
}

interface WorktreeDefaultResponse {
  provider: string;
}

export function useProvider() {
  const apiBase = `${resolveBackendUrl()}/api`;
  const providers = useState<string[]>('providers', () => []);
  const defaultProvider = useState<string>('default-provider', () => '');
  const loading = useState<boolean>('providers-loading', () => false);

  async function fetchProviders() {
    if (loading.value) return;
    loading.value = true;
    try {
      const data = await $fetch<ProvidersResponse>(`${apiBase}/providers`);
      providers.value = data.providers || [];
      defaultProvider.value = data.defaultProvider || '';
    } catch {
      console.error('Failed to fetch providers');
    } finally {
      loading.value = false;
    }
  }

  async function getWorktreeDefault(worktreePath: string): Promise<string> {
    try {
      const data = await $fetch<WorktreeDefaultResponse>(
        `${apiBase}/worktree-defaults?worktree=${encodeURIComponent(worktreePath)}`
      );
      return data.provider || defaultProvider.value;
    } catch {
      return defaultProvider.value;
    }
  }

  async function setWorktreeDefault(worktreePath: string, provider: string): Promise<boolean> {
    try {
      await $fetch(`${apiBase}/worktree-defaults`, {
        method: 'PUT',
        body: { worktreePath, provider },
      });
      return true;
    } catch {
      console.error('Failed to set worktree default provider');
      return false;
    }
  }

  // Fetch on first use if empty
  if (providers.value.length === 0) {
    fetchProviders();
  }

  return {
    providers: readonly(providers),
    defaultProvider: readonly(defaultProvider),
    loading: readonly(loading),
    fetchProviders,
    getWorktreeDefault,
    setWorktreeDefault,
  };
}
