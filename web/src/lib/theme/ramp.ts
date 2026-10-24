import { contrastRatio, oklchToHex } from './oklch';

export type ColorMode = 'light' | 'dark';

export interface RampContrastResult {
  ok: boolean;
  failures: string[];
}

export const DEFAULT_ACCENT_HUE = 192;

const ACCENT_STEP_COUNT = 12;
const TEXT_MIN_RATIO = 4.5;
const UI_MIN_RATIO = 3;
const GAMUT_EPSILON = 0.0001;
const CHROMA_STEP = 0.001;
const DARK_TEXT_ON_ACCENT = '#090d0d';

const SURFACE: Record<ColorMode, string> = {
  light: '#fbfdfd',
  dark: '#0e1313',
};

const ACCENT_STEPS: Record<ColorMode, readonly (readonly [number, number])[]> = {
  light: [
    [0.985, 0.012],
    [0.965, 0.025],
    [0.935, 0.045],
    [0.905, 0.065],
    [0.87, 0.08],
    [0.825, 0.09],
    [0.765, 0.1],
    [0.69, 0.11],
    [0.52, 0.105],
    [0.47, 0.1],
    [0.43, 0.09],
    [0.27, 0.05],
  ],
  dark: [
    [0.17, 0.015],
    [0.2, 0.025],
    [0.24, 0.04],
    [0.28, 0.055],
    [0.32, 0.065],
    [0.37, 0.075],
    [0.44, 0.085],
    [0.53, 0.095],
    [0.76, 0.11],
    [0.81, 0.1],
    [0.84, 0.09],
    [0.94, 0.04],
  ],
};

function isInSrgbGamut(lightness: number, chroma: number, hue: number): boolean {
  const radians = (hue * Math.PI) / 180;
  const a = chroma * Math.cos(radians);
  const b = chroma * Math.sin(radians);
  const lp = lightness + 0.3963377774 * a + 0.2158037573 * b;
  const mp = lightness - 0.1055613458 * a - 0.0638541728 * b;
  const sp = lightness - 0.0894841775 * a - 1.291485548 * b;
  const l = lp ** 3;
  const m = mp ** 3;
  const s = sp ** 3;
  const channels = [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
  return channels.every(
    (channel) => -GAMUT_EPSILON <= channel && 1 + GAMUT_EPSILON >= channel,
  );
}

function clampChromaToSrgb(lightness: number, chroma: number, hue: number): number {
  let clamped = chroma;
  while (0 < clamped && !isInSrgbGamut(lightness, clamped, hue)) {
    clamped -= CHROMA_STEP;
  }
  return Math.max(0, clamped);
}

export function accentRamp(hue: number, mode: ColorMode): string[] {
  return ACCENT_STEPS[mode].map(([lightness, chroma]) =>
    oklchToHex(lightness, clampChromaToSrgb(lightness, chroma, hue), hue),
  );
}

interface ContrastPair {
  id: string;
  foreground: string;
  background: string;
  minRatio: number;
}

function contrastPairs(ramp: readonly string[], mode: ColorMode): ContrastPair[] {
  const surface = SURFACE[mode];
  if ('light' === mode) {
    return [
      { id: 'text-on-accent', foreground: '#ffffff', background: ramp[8], minRatio: TEXT_MIN_RATIO },
      { id: 'accent-text', foreground: ramp[10], background: surface, minRatio: TEXT_MIN_RATIO },
      { id: 'accent-soft-text', foreground: ramp[10], background: ramp[1], minRatio: TEXT_MIN_RATIO },
      { id: 'focus-ring', foreground: ramp[8], background: surface, minRatio: UI_MIN_RATIO },
    ];
  }
  return [
    { id: 'text-on-accent', foreground: DARK_TEXT_ON_ACCENT, background: ramp[8], minRatio: TEXT_MIN_RATIO },
    { id: 'accent-text', foreground: ramp[10], background: surface, minRatio: TEXT_MIN_RATIO },
    { id: 'accent-soft-text', foreground: ramp[10], background: ramp[2], minRatio: TEXT_MIN_RATIO },
    { id: 'focus-ring', foreground: ramp[8], background: surface, minRatio: UI_MIN_RATIO },
  ];
}

export function checkRampContrast(ramp: readonly string[], mode: ColorMode): RampContrastResult {
  if (ACCENT_STEP_COUNT !== ramp.length) {
    return { ok: false, failures: ['ramp-length'] };
  }
  const failures = contrastPairs(ramp, mode)
    .filter((pair) => {
      const contrast = contrastRatio(pair.foreground, pair.background);
      return false === contrast.ok || !(pair.minRatio <= contrast.value);
    })
    .map((pair) => pair.id);
  return { ok: 0 === failures.length, failures };
}
