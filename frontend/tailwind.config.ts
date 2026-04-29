import type { Config } from 'tailwindcss'

function withOpacity(varName: string) {
  return `rgb(var(${varName}-rgb) / <alpha-value>)`;
}

export default <Partial<Config>>{
  content: [],
  theme: {
    extend: {
      colors: {
        'bg-primary': withOpacity('--bg-primary'),
        'bg-secondary': withOpacity('--bg-secondary'),
        'bg-elevated': withOpacity('--bg-elevated'),
        'text-primary': withOpacity('--text-primary'),
        'text-muted': withOpacity('--text-muted'),
        'accent': withOpacity('--accent'),
        'accent-secondary': withOpacity('--accent-secondary'),
        'accent-warn': withOpacity('--accent-warn'),
      },
    },
  },
}
