export interface Theme {
  bgPrimary: string;
  bgSecondary: string;
  bgElevated: string;
  textPrimary: string;
  textMuted: string;
  accent: string;
  accentSecondary: string;
  accentWarn: string;
}

export const defaultTheme: Theme = {
  bgPrimary: '#1A1A1A',
  bgSecondary: '#282828',
  bgElevated: '#3E3E3E',
  textPrimary: '#E5E5E5',
  textMuted: '#C6C6C6',
  accent: '#50BECF',
  accentSecondary: '#FFE194',
  accentWarn: '#FD6754',
};

const CSS_VAR_MAP: Record<keyof Theme, string> = {
  bgPrimary: '--bg-primary',
  bgSecondary: '--bg-secondary',
  bgElevated: '--bg-elevated',
  textPrimary: '--text-primary',
  textMuted: '--text-muted',
  accent: '--accent',
  accentSecondary: '--accent-secondary',
  accentWarn: '--accent-warn',
};

export function applyTheme(theme: Theme = defaultTheme): void {
  const root = document.documentElement;
  for (const [key, cssVar] of Object.entries(CSS_VAR_MAP)) {
    const hex = theme[key as keyof Theme];
    root.style.setProperty(cssVar, hex);
    // Set RGB components for Tailwind opacity modifier support
    root.style.setProperty(`${cssVar}-rgb`, hexToRgb(hex));
  }
}

function hexToRgb(hex: string): string {
  const h = hex.replace('#', '');
  const r = parseInt(h.substring(0, 2), 16);
  const g = parseInt(h.substring(2, 4), 16);
  const b = parseInt(h.substring(4, 6), 16);
  return `${r} ${g} ${b}`;
}

export function useTheme() {
  const currentTheme = useState<Theme>('theme', () => defaultTheme);

  function setTheme(theme: Theme) {
    currentTheme.value = theme;
    if (import.meta.client) {
      applyTheme(theme);
    }
  }

  return {
    currentTheme: readonly(currentTheme),
    setTheme,
  };
}
