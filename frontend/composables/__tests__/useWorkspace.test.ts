import { describe, it, expect, beforeEach, vi } from 'vitest';
import { ref, readonly } from 'vue';

// Provide Nuxt auto-imports as globals
vi.stubGlobal('ref', ref);
vi.stubGlobal('readonly', readonly);
vi.stubGlobal('useState', (_key: string, init?: () => unknown) => ref(init ? init() : undefined));

// Mock resolveBackendUrl
vi.stubGlobal('resolveBackendUrl', () => 'http://localhost:8080');

// Mock $fetch
const mockFetch = vi.fn();
vi.stubGlobal('$fetch', mockFetch);

import { useWorkspace } from '../useWorkspace';
import type { RemoveError } from '../useWorkspace';

beforeEach(() => {
  mockFetch.mockReset();
  // useWorkspace auto-fetches projects on first use; always resolve that call.
  mockFetch.mockResolvedValueOnce([]);
});

describe('removeWorktree', () => {
  it('returns null on success and refreshes projects', async () => {
    mockFetch
      .mockResolvedValueOnce(undefined) // DELETE call
      .mockResolvedValueOnce([]); // fetchProjects

    const { removeWorktree } = useWorkspace();
    const result = await removeWorktree('myproject', 'feature-branch');

    expect(result).toBeNull();
    expect(mockFetch).toHaveBeenCalledWith(
      'http://localhost:8080/api/projects/myproject/worktrees/feature-branch',
      { method: 'DELETE' },
    );
  });

  it('returns RemoveError with forceable=true on 409', async () => {
    const errorData: RemoveError = {
      error: 'worktree has uncommitted changes',
      forceable: true,
      files: [
        { path: 'src/main.ts', status: 'modified' },
        { path: 'temp.log', status: 'untracked' },
      ],
    };
    mockFetch.mockRejectedValueOnce({ data: errorData });

    const { removeWorktree } = useWorkspace();
    const result = await removeWorktree('myproject', 'dirty-branch');

    expect(result).toEqual(errorData);
  });

  it('returns RemoveError with forceable=false for generic errors', async () => {
    mockFetch.mockRejectedValueOnce({ data: { error: 'worktree not found', forceable: false } });

    const { removeWorktree } = useWorkspace();
    const result = await removeWorktree('myproject', 'gone');

    expect(result).toEqual({ error: 'worktree not found', forceable: false, files: undefined });
  });

  it('passes force=true as query param', async () => {
    mockFetch
      .mockResolvedValueOnce(undefined)
      .mockResolvedValueOnce([]);

    const { removeWorktree } = useWorkspace();
    await removeWorktree('myproject', 'dirty-branch', true);

    expect(mockFetch).toHaveBeenCalledWith(
      'http://localhost:8080/api/projects/myproject/worktrees/dirty-branch?force=true',
      { method: 'DELETE' },
    );
  });

  it('returns fallback error when response has no data', async () => {
    mockFetch.mockRejectedValueOnce(new Error('network error'));

    const { removeWorktree } = useWorkspace();
    const result = await removeWorktree('myproject', 'branch');

    expect(result).toEqual({ error: 'Failed to remove worktree', forceable: false, files: undefined });
  });
});
