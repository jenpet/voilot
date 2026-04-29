# Plan: Color Theming System

**Status:** draft
**Created:** 2026-04-29
**Author:** jenpet + planitect

## Goal

Replace the hardcoded Tailwind `surface` color scale with a semantic token-based theming system backed by CSS custom properties. The default theme uses colors extracted from jen.pet. The architecture supports multiple themes and future user configuration via settings, but no UI is exposed yet.

## Context

- The frontend currently defines an 11-shade `surface` scale (50-950) in `tailwind.config.ts`
- There are ~129 references to `surface-*` utility classes across Vue components
- Additional hardcoded Tailwind colors (`bg-green-600`, `bg-blue-500`, `text-amber-200`, `text-red-400`, etc.) are used for accents, states, and focus rings
- All of these need to migrate to semantic tokens so themes can be swapped at runtime by updating CSS variables

## Design Tokens

8 semantic tokens, derived from the jen.pet palette:

| Token              | Default (hex) | Usage                                         |
|--------------------|---------------|-----------------------------------------------|
| `bg-primary`       | `#282828`     | Main app background, shell                    |
| `bg-secondary`     | `#3E3E3E`     | Cards, panels, input fields, chat bubbles     |
| `bg-elevated`      | `#4A4A4A`     | Hover states, borders, dividers               |
| `text-primary`     | `#E5E5E5`     | Main content text                             |
| `text-muted`       | `#C6C6C6`     | Labels, timestamps, secondary info            |
| `accent`           | `#50BECF`     | Buttons, links, focus rings, active states, success toggles, info |
| `accent-secondary` | `#FFE194`     | Badges, mode indicators                       |
| `accent-warn`      | `#FD6754`     | Errors, destructive actions, recording state  |

Borders and dividers reuse `bg-elevated` rather than having a dedicated token.

## Approach

### 1. Create a theme definition module

Create `frontend/composables/useTheme.ts` (or `frontend/utils/theme.ts`) containing:

- A `Theme` type defining the 8 token keys and their color values
- A `defaultTheme` object with the jen.pet colors above
- A function that applies a theme by setting CSS custom properties on `document.documentElement`
- The theme is applied on app startup (Nuxt plugin or `app.vue` onMounted)

```ts
// Conceptual structure
interface Theme {
  bgPrimary: string;
  bgSecondary: string;
  bgElevated: string;
  textPrimary: string;
  textMuted: string;
  accent: string;
  accentSecondary: string;
  accentWarn: string;
}
```

### 2. Wire Tailwind to CSS custom properties

Update `tailwind.config.ts` to replace the `surface` scale with the 8 semantic tokens, each pointing at CSS variables:

```ts
colors: {
  'bg-primary': 'var(--bg-primary)',
  'bg-secondary': 'var(--bg-secondary)',
  'bg-elevated': 'var(--bg-elevated)',
  'text-primary': 'var(--text-primary)',
  'text-muted': 'var(--text-muted)',
  'accent': 'var(--accent)',
  'accent-secondary': 'var(--accent-secondary)',
  'accent-warn': 'var(--accent-warn)',
}
```

### 3. Migrate all component references

Replace all `surface-*` and hardcoded color classes across components:

- `bg-surface-900`, `bg-surface-800` -> `bg-bg-primary` or `bg-bg-secondary`
- `bg-surface-700`, `bg-surface-600` -> `bg-bg-secondary` or `bg-bg-elevated`
- `text-surface-100`, `text-surface-200`, `text-surface-300` -> `text-text-primary`
- `text-surface-400`, `text-surface-500` -> `text-text-muted`
- `border-surface-*` -> `border-bg-elevated`
- `bg-green-600` (toggles) -> `bg-accent`
- `bg-blue-500`, `ring-blue-500` (focus) -> `bg-accent` / `ring-accent`
- `text-red-400`, `bg-red-*` (errors) -> `text-accent-warn` / `bg-accent-warn`
- `text-amber-*` (warnings) -> `text-accent-secondary`
- `text-indigo-*` (info) -> `text-accent`

### 4. Update PWA theme color

Update `nuxt.config.ts` to set `theme_color` to `#282828` (currently `#0f172a`).

## Open Questions

None -- all decisions resolved.

## Acceptance Criteria

- All hardcoded color values are removed from component templates; only semantic token classes are used
- The `defaultTheme` object is the single source of truth for the jen.pet color palette
- Changing a value in the theme object and reloading reflects across the entire app
- No visual regressions -- the app looks intentionally themed, not broken
- The PWA theme color matches `bg-primary`
