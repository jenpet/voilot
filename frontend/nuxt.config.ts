// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-01-01',

  devtools: { enabled: true },

  devServer: {
    port: 3000,
  },

  modules: [
    '@nuxtjs/tailwindcss',
    // '@vite-pwa/nuxt', // Disabled during development — SW caching serves stale JS on mobile
  ],

  // No dev proxy — in development the frontend talks directly to the Go backend
  // via the backendUrl runtime config (default: http://localhost:8080).
  // This avoids ECONNRESET crashes when the backend is unavailable.
  // In production, Nginx proxies /api and /ws to the backend.

  // Allow Tailscale MagicDNS hostnames through Vite's host check (dev only)
  vite: {
    server: {
      allowedHosts: ['.ts.net'],
    },
  },

  // Static site generation for production (served by nginx)
  ssr: false,

  // PWA disabled during development — re-enable @vite-pwa/nuxt in modules[] when ready
  // pwa: { ... },

  // Global CSS
  css: ['~/assets/css/markdown.css'],

  // App configuration
  app: {
    head: {
      title: 'voilot',
      meta: [
        { name: 'viewport', content: 'width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no' },
        { name: 'theme-color', content: '#0f172a' },
        { name: 'apple-mobile-web-app-capable', content: 'yes' },
        { name: 'apple-mobile-web-app-status-bar-style', content: 'black-translucent' },
      ],
    },
  },

  // Runtime config (environment variables)
  // In dev: API goes through Nitro devProxy (/api -> localhost:8080),
  // but WebSocket connects directly to the Go backend (no proxy) to
  // avoid ECONNRESET crashes when the backend is unavailable.
  // In production: Nginx proxies both /api and /ws to the backend.
  runtimeConfig: {
    public: {
      // Base URL of the Go backend. In dev: direct connection.
      // In production behind Nginx, set to '' (empty) so relative paths work.
      // Override with NUXT_PUBLIC_BACKEND_URL env var.
      backendUrl: 'http://localhost:8080',
    },
  },
})
