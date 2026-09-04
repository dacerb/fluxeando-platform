/**
 * Fuente única de las paletas oscuras. Los nombres expresan intención; las
 * vistas consumen las variables --m3-* / --ui-* que se generan desde aquí.
 */
export type DarkPaletteName = 'graphite' | 'gold';

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
  gold: {
    name: { es: 'Oro y negro', en: 'Gold and black' },
    canvas: '#121212', surface: '#1A1A1A', surfaceHigh: '#262626',
    text: '#FFFFFF', muted: '#A3A3A3', accent: '#FEBE10', onAccent: '#1A1A1A',
    swatches: ['#FFFFFF', '#F2F2F2', '#FEBE10', '#1A1A1A']
  }
};
