/**
 * PWA prompt state management.
 *
 * Wraps @vite-pwa/nuxt's `usePWA()` composable and adds mobile/iOS detection
 * to drive a unified install/update overlay on the landing page.
 *
 * Uses module-level shared state so all callers (component + page) see the
 * same promptType, canShowInstall, etc.
 *
 * Four states for `promptType`:
 *   - 'install-android' — mobile Chrome/Android, native install available
 *   - 'install-ios'     — mobile iOS Safari, manual "Add to Home Screen" instructions
 *   - 'update'          — new SW waiting, show "Update now" prompt
 *   - null              — hidden (desktop, dismissed, or standalone with no update)
 */

const DISMISS_KEY = 'voilot-pwa-install-dismissed';

type PromptType = 'install-android' | 'install-ios' | 'update' | null;

// ── Shared module-level state ───────────────────────────────────────
const _promptType = ref<PromptType>(null);
const _standalone = ref(false);
const _canShowInstall = ref(false);
let _initialized = false;

// ── Detection helpers ───────────────────────────────────────────────

function isMobile(): boolean {
  if (typeof navigator === 'undefined') return false;
  return /Android|iPhone|iPad|iPod/i.test(navigator.userAgent)
    || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
}

function isIOS(): boolean {
  if (typeof navigator === 'undefined') return false;
  return /iPhone|iPad|iPod/i.test(navigator.userAgent)
    || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1);
}

function isStandaloneMode(): boolean {
  if (typeof window === 'undefined') return false;
  return window.matchMedia('(display-mode: standalone)').matches
    || (navigator as any).standalone === true;
}

function wasDismissed(): boolean {
  if (typeof localStorage === 'undefined') return false;
  return localStorage.getItem(DISMISS_KEY) === 'true';
}

// ── Composable ──────────────────────────────────────────────────────

export function usePwaPrompt() {
  // SSR guard — return inert refs
  if (!import.meta.client) {
    return {
      promptType: readonly(_promptType),
      isStandalone: readonly(_standalone),
      canShowInstall: readonly(_canShowInstall),
      triggerInstall: async () => {},
      triggerUpdate: () => {},
      dismiss: () => {},
      reopen: () => {},
    };
  }

  // Only initialize once across all callers
  if (!_initialized) {
    _initialized = true;
    _init();
  }

  return {
    promptType: readonly(_promptType),
    isStandalone: readonly(_standalone),
    canShowInstall: readonly(_canShowInstall),
    triggerInstall,
    triggerUpdate,
    dismiss,
    reopen,
  };
}

// ── Private: one-time initialization ────────────────────────────────

let _pwa: ReturnType<typeof useNuxtApp>['$pwa'];

function _init() {
  const { $pwa } = useNuxtApp();
  _pwa = $pwa;

  _standalone.value = isStandaloneMode();

  console.log('[pwa-prompt] init', {
    hasPwa: !!$pwa,
    standalone: _standalone.value,
    mobile: isMobile(),
    ios: isIOS(),
    ua: typeof navigator !== 'undefined' ? navigator.userAgent : 'n/a',
  });

  // Watch for needRefresh changes (SW update detected)
  if ($pwa) {
    watch(() => $pwa.needRefresh, (val) => {
      if (val) {
        _promptType.value = 'update';
      }
    });

    watch(() => $pwa.showInstallPrompt, () => {
      _evaluate();
    });
  }

  // Initial evaluation
  _evaluate();
}

function _evaluate() {
  const mobile = isMobile();
  const ios = isIOS();
  const dismissed = wasDismissed();

  console.log('[pwa-prompt] evaluate', {
    mobile, ios, dismissed,
    standalone: _standalone.value,
    hasPwa: !!_pwa,
    needRefresh: _pwa?.needRefresh,
    isPWAInstalled: _pwa?.isPWAInstalled,
    showInstallPrompt: _pwa?.showInstallPrompt,
  });

  // Update takes priority over install
  if (_pwa?.needRefresh) {
    _promptType.value = 'update';
    _canShowInstall.value = false;
    console.log('[pwa-prompt] result: update');
    return;
  }

  // Already installed — no install prompt
  if (_standalone.value || _pwa?.isPWAInstalled) {
    _promptType.value = null;
    _canShowInstall.value = false;
    console.log('[pwa-prompt] result: null (standalone)');
    return;
  }

  // Track whether install could be shown (for the header re-open button)
  _canShowInstall.value = mobile;

  // Not mobile — no prompt
  if (!mobile) {
    _promptType.value = null;
    console.log('[pwa-prompt] result: null (not mobile)');
    return;
  }

  // Previously dismissed — no prompt (but canShowInstall stays true)
  if (dismissed) {
    _promptType.value = null;
    console.log('[pwa-prompt] result: null (dismissed)');
    return;
  }

  // Mobile, not installed, not dismissed
  if (ios) {
    _promptType.value = 'install-ios';
  } else if (_pwa?.showInstallPrompt) {
    _promptType.value = 'install-android';
  } else {
    // Android but no beforeinstallprompt captured (yet) — hide for now
    _promptType.value = null;
  }

  console.log('[pwa-prompt] result:', _promptType.value);
}

// ── Actions ─────────────────────────────────────────────────────────

async function triggerInstall() {
  if (!_pwa) return;
  const result = await _pwa.install();
  if (result?.outcome === 'accepted') {
    _promptType.value = null;
  }
}

function triggerUpdate() {
  _pwa?.updateServiceWorker(true);
}

function dismiss() {
  if (_promptType.value?.startsWith('install')) {
    localStorage.setItem(DISMISS_KEY, 'true');
  }
  _promptType.value = null;
}

/** Re-open the install overlay (clears the dismiss flag and re-evaluates). */
function reopen() {
  localStorage.removeItem(DISMISS_KEY);
  _evaluate();
}
