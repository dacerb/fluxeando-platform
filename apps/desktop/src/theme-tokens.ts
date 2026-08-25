/**
 * Fuente única de las paletas oscuras. Los nombres expresan intención; las
 * vistas consumen las variables --m3-* / --ui-* que se generan desde aquí.
 */
export type DarkPaletteName = 'graphite' | 'ocean' | 'violet';

export type DarkPalette = {
  name: { es: string; en: string };
  canvas: string;
  surface: string;
  surfaceHigh: string;
  text: string;
  muted: string;
  accent: string;
  onAccent: string;
  swatches: string[];
};

export const darkPalettes: Record<DarkPaletteName, DarkPalette> = {
  graphite: {
    name: { es: 'Naranja cálido', en: 'Warm orange' },
    canvas: '#181818', surface: '#252525', surfaceHigh: '#303030',
    text: '#FFFFFF', muted: '#B8B8B8', accent: '#FA5F1A', onAccent: '#252525',
    swatches: ['#181818', '#252525', '#FFFFFF', '#FA5F1A']
  },
  ocean: {
    name: { es: 'Pizarra y arena', en: 'Slate and sand' },
    canvas: '#20262e', surface: '#708090', surfaceHigh: '#708090',
    text: '#ffffff', muted: '#b8c0cc', accent: '#9b8c7a', onAccent: '#ffffff',
    swatches: ['#708090', '#20262e', '#b8c0cc', '#ffffff', '#9b8c7a']
  },
  violet: {
    name: { es: 'Carbón y naranja', en: 'Midnight and orange' },
    canvas: '#161616', surface: '#E4E2E3', surfaceHigh: '#A8AAAC',
    text: '#FEF8E8', muted: '#A8AAAC', accent: '#F44A22', onAccent: '#FEF8E8',
    swatches: ['#161616', '#FEF8E8', '#E4E2E3', '#A8AAAC', '#F44A22']
  }
};
