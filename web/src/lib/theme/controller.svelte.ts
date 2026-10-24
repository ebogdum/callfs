import { accentRamp, DEFAULT_ACCENT_HUE, type ColorMode } from './ramp';

export type ThemeMode = 'system' | 'light' | 'dark';

export interface ThemePrefs {
  theme: ThemeMode;
  contrast: 'standard' | 'more';
  density: 'compact' | 'default' | 'comfortable';
  motion: 'full' | 'reduced' | 'none';
  fontScale: number;
  accentHue: number;
  radius: 'sharp' | 'medium' | 'round';
}

const DARK_QUERY = '(prefers-color-scheme: dark)';
const SWITCH_ATTRIBUTE = 'data-theme-switching';
const ACCENT_PROPERTIES = Array.from({ length: 12 }, (_, index) => `--a-${index + 1}`);
const BACKGROUND_HEX: Record<ThemePrefs['contrast'], Record<ColorMode, string>> = {
  standard: { light: '#f5f9f9', dark: '#090d0d' },
  more: { light: '#ffffff', dark: '#000000' },
};

function isColorMode(value: string | undefined): value is ColorMode {
  return 'light' === value || 'dark' === value;
}

export function resolveColorMode(theme: ThemeMode, prefersDark: boolean): ColorMode {
  if ('light' === theme) {
    return 'light';
  }
  if ('dark' === theme) {
    return 'dark';
  }
  return prefersDark ? 'dark' : 'light';
}

export class ThemeController {
  readonly #root: HTMLElement;
  readonly #media: MediaQueryList | null;
  readonly #observer: MutationObserver;
  #prefs: ThemePrefs | null = null;
  #accent: Record<ColorMode, string[]> | null = null;

  readonly #onSystemChange = (): void => {
    if (null !== this.#prefs && 'system' === this.#prefs.theme) {
      this.apply(this.#prefs);
    }
  };

  constructor(root: HTMLElement = document.documentElement) {
    this.#root = root;
    this.#media = 'function' === typeof window.matchMedia ? window.matchMedia(DARK_QUERY) : null;
    this.#media?.addEventListener('change', this.#onSystemChange);
    this.#observer = new MutationObserver(() => this.#writeAccent());
    this.#observer.observe(root, { attributes: true, attributeFilter: ['data-theme'] });
  }

  apply(prefs: ThemePrefs): void {
    this.#prefs = prefs;
    const root = this.#root;
    const mode = resolveColorMode(prefs.theme, Boolean(this.#media?.matches));
    if (mode !== root.dataset.theme) {
      root.setAttribute(SWITCH_ATTRIBUTE, '');
      window.requestAnimationFrame(() => root.removeAttribute(SWITCH_ATTRIBUTE));
    }
    root.dataset.theme = mode;
    root.dataset.density = prefs.density;
    root.dataset.contrast = prefs.contrast;
    root.dataset.motion = prefs.motion;
    root.dataset.radius = prefs.radius;
    root.style.setProperty('--font-scale', String(prefs.fontScale));
    this.#accent =
      DEFAULT_ACCENT_HUE === prefs.accentHue
        ? null
        : { light: accentRamp(prefs.accentHue, 'light'), dark: accentRamp(prefs.accentHue, 'dark') };
    this.#writeAccent();
    this.#themeColorMeta().content = BACKGROUND_HEX[prefs.contrast][mode];
  }

  dispose(): void {
    this.#media?.removeEventListener('change', this.#onSystemChange);
    this.#observer.disconnect();
  }

  #writeAccent(): void {
    const mode = this.#root.dataset.theme;
    if (!isColorMode(mode)) {
      return;
    }
    ACCENT_PROPERTIES.forEach((property, index) => {
      if (null === this.#accent) {
        this.#root.style.removeProperty(property);
        return;
      }
      this.#root.style.setProperty(property, this.#accent[mode][index] ?? '');
    });
  }

  #themeColorMeta(): HTMLMetaElement {
    const document = this.#root.ownerDocument;
    const existing = document.querySelector('meta[name="theme-color"]');
    if (existing instanceof HTMLMetaElement) {
      return existing;
    }
    const meta = document.createElement('meta');
    meta.name = 'theme-color';
    document.head.append(meta);
    return meta;
  }
}
