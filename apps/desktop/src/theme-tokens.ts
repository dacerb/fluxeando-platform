/**
 * Fuente única de las paletas oscuras. Los nombres expresan intención; las
 * vistas consumen las variables --m3-* / --ui-* que se generan desde aquí.
 */
export type DarkPaletteName = 'graphite' | 'gold' | 'neon' | 'rose';

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
  },
  neon: {
    name: { es: 'Neón y obsidiana', en: 'Neon and obsidian' },
    canvas: '#101010', surface: '#1B1B1B', surfaceHigh: '#272727',
    text: '#F4F4F4', muted: '#A1A1A1', accent: '#EEFF22', onAccent: '#101010',
    swatches: ['#EEFF22', '#101010', '#D6D6D6', '#F4F4F4']
  },
  rose: {
    name: { es: 'Rosa y pizarra', en: 'Rose and slate' },
    canvas: '#141C20', surface: '#202A30', surfaceHigh: '#2A363D',
    text: '#E5D4C8', muted: '#9BA5AA', accent: '#FF4777', onAccent: '#182126',
    swatches: ['#FF4777', '#36434A', '#E5D4C8']
  }
};
