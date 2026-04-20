export interface Project {
  name: string;
  path: string;
  worktrees: Worktree[];
}

export interface Worktree {
  name: string;
  path: string;
  branch: string;
  isMain: boolean;
}

export function useWorkspace() {
  const projects = useState<Project[]>('workspace-projects', () => []);
  const loading = useState('workspace-loading', () => false);
  const apiBase = `${resolveBackendUrl()}/api`;

  async function fetchProjects() {
    loading.value = true;
    try {
      const data = await $fetch<Project[]>(`${apiBase}/projects`);
      projects.value = data || [];
    } catch {
      // Workspace not configured — leave empty
      projects.value = [];
    } finally {
      loading.value = false;
    }
  }

  async function createWorktree(projectName: string, description: string): Promise<Project | null> {
    try {
      const project = await $fetch<Project>(`${apiBase}/projects/${projectName}/worktrees`, {
        method: 'POST',
        body: { description },
      });
      // Refresh projects list
      await fetchProjects();
      return project;
    } catch {
      console.error('Failed to create worktree');
      return null;
    }
  }

  async function removeWorktree(projectName: string, worktreeName: string): Promise<boolean> {
    try {
      await $fetch(`${apiBase}/projects/${projectName}/worktrees/${worktreeName}`, {
        method: 'DELETE',
      });
      await fetchProjects();
      return true;
    } catch {
      console.error('Failed to remove worktree');
      return false;
    }
  }

  async function addProject(repoPath: string): Promise<string | null> {
    try {
      await $fetch(`${apiBase}/projects`, {
        method: 'POST',
        body: { path: repoPath },
      });
      await fetchProjects();
      return null;
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Failed to add project';
      return msg;
    }
  }

  async function cloneProject(url: string, name?: string): Promise<string | null> {
    try {
      await $fetch(`${apiBase}/projects/clone`, {
        method: 'POST',
        body: { url, name: name || undefined },
      });
      await fetchProjects();
      return null;
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Failed to clone project';
      return msg;
    }
  }

  async function initProject(name: string): Promise<string | null> {
    try {
      await $fetch(`${apiBase}/projects/init`, {
        method: 'POST',
        body: { name },
      });
      await fetchProjects();
      return null;
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : 'Failed to create project';
      return msg;
    }
  }

  async function fetchWorktreeSessions(worktreePath: string): Promise<string[]> {
    try {
      const data = await $fetch<string[]>(`${apiBase}/worktrees/${encodeURIComponent(worktreePath)}/sessions`);
      return data || [];
    } catch {
      return [];
    }
  }

  // Fetch on first use
  if (projects.value.length === 0) {
    fetchProjects();
  }

  return {
    projects: readonly(projects),
    loading: readonly(loading),
    fetchProjects,
    addProject,
    cloneProject,
    initProject,
    createWorktree,
    removeWorktree,
    fetchWorktreeSessions,
  };
}
