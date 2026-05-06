// https://nuxt.com/docs/api/configuration/nuxt-config

const isDev = process.env.NODE_ENV !== 'production';
const appName = isDev ? 'voilot dev' : 'voilot';
const iconPrefix = isDev ? '/icons/dev' : '/icons';

export default defineNuxtConfig({
  compatibilityDate: '2025-01-01',

  devtools: { enabled: false },

  devServer: {
    port: 3000,
  },

  modules: [
    '@nuxtjs/tailwindcss',
    '@vite-pwa/nuxt',
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

  // PWA configuration — environment-aware caching and manifest
  pwa: {
    registerType: isDev ? 'autoUpdate' : 'prompt',
    manifest: {
      name: appName,
      short_name: appName,
      start_url: '/',
      display: 'standalone',
      background_color: '#1A1A1A',
      theme_color: '#1A1A1A',
      icons: [
        { src: `${iconPrefix}/icon-192x192.png`, sizes: '192x192', type: 'image/png' },
        { src: `${iconPrefix}/icon-512x512.png`, sizes: '512x512', type: 'image/png' },
        { src: `${iconPrefix}/icon-maskable-512x512.png`, sizes: '512x512', type: 'image/png', purpose: 'maskable' },
      ],
    },
    workbox: {
      // Never cache API or WebSocket traffic
      navigateFallback: '/',
      navigateFallbackDenylist: [/^\/api/, /^\/ws/],
      runtimeCaching: [
        {
          urlPattern: /^https?.*/,
          handler: 'NetworkFirst',
          options: {
            cacheName: 'app-shell',
            expiration: { maxEntries: 50, maxAgeSeconds: 30 * 24 * 60 * 60 },
            networkTimeoutSeconds: 3,
          },
        },
        {
          urlPattern: /\.(?:js|css|woff2?|png|jpg|jpeg|svg|gif|webp|wav|mp3|ogg)$/,
          handler: 'CacheFirst',
          options: {
            cacheName: 'static-assets',
            expiration: { maxEntries: 100, maxAgeSeconds: 30 * 24 * 60 * 60 },
          },
        },
      ],
      // Include audio feedback files in precache
      maximumFileSizeToCacheInBytes: 5 * 1024 * 1024,
      skipWaiting: isDev,
      clientsClaim: true,
    },
    devOptions: {
      enabled: false,
    },
  },

  // Global CSS
  css: ['~/assets/css/global.css', '~/assets/css/markdown.css'],

  // App configuration
  app: {
    head: {
      htmlAttrs: { lang: 'en' },
      title: appName,
      meta: [
        { name: 'viewport', content: 'width=device-width, initial-scale=1, viewport-fit=cover' },
        { name: 'description', content: 'voilot — voice-first AI coding agent client' },
        { name: 'theme-color', content: '#1A1A1A' },
        { name: 'apple-mobile-web-app-capable', content: 'yes' },
        { name: 'apple-mobile-web-app-status-bar-style', content: 'black-translucent' },
        { name: 'apple-mobile-web-app-title', content: appName },
        { property: 'og:title', content: appName },
        { property: 'og:description', content: 'voilot — voice-first AI coding agent client' },
        { property: 'og:type', content: 'website' },
      ],
      link: [
        { rel: 'icon', type: 'image/svg+xml', href: isDev ? '/favicon-dev.svg' : '/favicon.svg' },
        { rel: 'apple-touch-icon', href: `${iconPrefix}/apple-touch-icon.png` },
      ],
    },
  },

  // Runtime config (environment variables)
  // In dev: the frontend talks directly to the Go backend.
  // In production behind Nginx, set to '' (empty) so relative paths work.
  runtimeConfig: {
    public: {
      // Base URL of the Go backend. In dev: direct connection.
      // In production behind Nginx, set to '' (empty) so relative paths work.
      // Override with NUXT_PUBLIC_BACKEND_URL env var.
      backendUrl: 'http://localhost:8080',
    },
  },
})
