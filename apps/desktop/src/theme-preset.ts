import type { DarkPaletteName } from './theme-tokens';

/** La única combinación habilitada mientras se calibra el sistema visual. */
export const activeThemePreset = {
  lightAccent: 'graphite',
  darkAccent: 'graphite' as DarkPaletteName,
  maxPaletteOptions: 4
} as const;
