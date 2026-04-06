/**
 * Resolves the backend URL based on the current browser context.
 *
 * In dev mode, nuxt.config sets backendUrl to "http://localhost:8080".
 * This works when browsing from localhost, but breaks when accessing
 * the frontend from another device (e.g., phone on the same network)
 * because the browser tries to reach localhost on the phone itself.
 *
 * This utility detects that situation and replaces "localhost" with the
 * actual hostname the browser is using, so the backend is reachable
 * from any device on the network (Go binds to 0.0.0.0 by default).
 *
 * In production (backendUrl is empty), returns '' so relative paths
 * work behind nginx — no change in behavior.
 */
export function resolveBackendUrl(): string {
  if (typeof window === 'undefined') return '';

  const config = useRuntimeConfig();
  const configured = (config.public.backendUrl as string) || '';

  // Empty = production behind nginx, use relative paths
  if (!configured) return '';

  // If the browser is on localhost, the configured URL works as-is
  const browserHost = window.location.hostname;
  if (browserHost === 'localhost' || browserHost === '127.0.0.1') {
    return configured;
  }

  // Browser is on a different host (e.g., phone via 192.168.x.x or Tailscale *.ts.net).
  // Replace localhost in the configured URL with the browser's hostname
  // so requests go to the same machine that served the frontend.
  // Also adopt the page's protocol so HTTPS pages don't make mixed-content HTTP requests.
  try {
    const url = new URL(configured);
    if (url.hostname === 'localhost' || url.hostname === '127.0.0.1') {
      url.hostname = browserHost;
      url.protocol = window.location.protocol;
      // Remove trailing slash for consistency
      return url.origin + url.pathname.replace(/\/$/, '');
    }
  } catch {
    // Malformed URL — fall through
  }

  return configured;
}
