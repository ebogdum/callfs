import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { oklchToHex } from './oklch';
import { ThemeController, type ThemePrefs } from './controller.svelte';
import { accentRamp, checkRampContrast } from './ramp';

const HEX_PATTERN = /^#[0-9a-f]{6}$/i;

const basePrefs: ThemePrefs = {
  theme: 'light',
  contrast: 'standard',
  density: 'default',
  motion: 'full',
  fontScale: 1,
  accentHue: 192,
  radius: 'medium',
};

describe('accentRamp', () => {
  it('returns twelve hex steps for light and dark', () => {
    for (const mode of ['light', 'dark'] as const) {
      const ramp = accentRamp(192, mode);
      expect(ramp).toHaveLength(12);
      for (const step of ramp) {
        expect(step).toMatch(HEX_PATTERN);
      }
    }
  });

  it('uses the lightness of step 9 and clamps its chroma into the sRGB gamut', () => {
    expect(accentRamp(192, 'light')[8]).toBe('#017976');
    expect(accentRamp(192, 'dark')[8]).toBe(oklchToHex(0.76, 0.11, 192));
  });

  it('returns valid hex for hues across the wheel', () => {
    for (const hue of [0, 60, 120, 240, 300, 359]) {
      for (const mode of ['light', 'dark'] as const) {
        for (const step of accentRamp(hue, mode)) {
          expect(step).toMatch(HEX_PATTERN);
        }
      }
    }
  });
});

describe('checkRampContrast', () => {
  it('passes the default accent ramp in light', () => {
    expect(checkRampContrast(accentRamp(192, 'light'), 'light')).toEqual({ ok: true, failures: [] });
  });

  it('passes the default accent ramp in dark', () => {
    expect(checkRampContrast(accentRamp(192, 'dark'), 'dark')).toEqual({ ok: true, failures: [] });
  });

  it('fails only the soft accent pair for a black ramp in light', () => {
    const ramp = Array.from({ length: 12 }, () => '#000000');
    expect(checkRampContrast(ramp, 'light')).toEqual({ ok: false, failures: ['accent-soft-text'] });
  });

  it('names every failing pair for a white ramp in light', () => {
    const ramp = Array.from({ length: 12 }, () => '#ffffff');
    const result = checkRampContrast(ramp, 'light');
    expect(result.ok).toBe(false);
    expect(result.failures).toContain('text-on-accent');
    expect(result.failures).toContain('accent-text');
  });

  it('names every failing pair for a black ramp in dark', () => {
    const ramp = Array.from({ length: 12 }, () => '#000000');
    const result = checkRampContrast(ramp, 'dark');
    expect(result.ok).toBe(false);
    expect(result.failures).toContain('text-on-accent');
    expect(result.failures).toContain('accent-text');
  });

  it('rejects a ramp that does not have twelve steps', () => {
    expect(checkRampContrast(['#000000'], 'light')).toEqual({ ok: false, failures: ['ramp-length'] });
  });
});

describe('ThemeController', () => {
  const root = document.documentElement;
  const mediaListeners = new Set<() => void>();
  let controller: ThemeController | null = null;
  let systemDark = false;

  beforeEach(() => {
    systemDark = false;
    mediaListeners.clear();
    vi.stubGlobal('matchMedia', () => ({
      get matches() {
        return systemDark;
      },
      addEventListener: (_type: string, listener: () => void) => {
        mediaListeners.add(listener);
      },
      removeEventListener: (_type: string, listener: () => void) => {
        mediaListeners.delete(listener);
      },
    }));
    const meta = document.createElement('meta');
    meta.name = 'theme-color';
    document.head.append(meta);
  });

  afterEach(() => {
    controller?.dispose();
    controller = null;
    vi.unstubAllGlobals();
    for (const name of root.getAttributeNames()) {
      if (name.startsWith('data-')) {
        root.removeAttribute(name);
      }
    }
    root.removeAttribute('style');
    document.querySelectorAll('meta[name="theme-color"]').forEach((meta) => meta.remove());
  });

  it('writes data attributes, font scale and theme-color', () => {
    controller = new ThemeController(root);
    controller.apply({ ...basePrefs, density: 'compact', motion: 'reduced', radius: 'round', contrast: 'more' });
    expect(root.dataset.theme).toBe('light');
    expect(root.dataset.density).toBe('compact');
    expect(root.dataset.contrast).toBe('more');
    expect(root.dataset.motion).toBe('reduced');
    expect(root.dataset.radius).toBe('round');
    expect(root.style.getPropertyValue('--font-scale')).toBe('1');
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute('content')).toBe('#ffffff');
  });

  it('writes accent overrides for a custom hue and switches them with data-theme', async () => {
    controller = new ThemeController(root);
    controller.apply({ ...basePrefs, accentHue: 60 });
    expect(root.style.getPropertyValue('--a-9')).toBe(accentRamp(60, 'light')[8]);
    root.setAttribute('data-theme', 'dark');
    await Promise.resolve();
    expect(root.style.getPropertyValue('--a-9')).toBe(accentRamp(60, 'dark')[8]);
  });

  it('removes accent overrides at the default hue', () => {
    controller = new ThemeController(root);
    controller.apply({ ...basePrefs, accentHue: 60 });
    controller.apply({ ...basePrefs, accentHue: 192 });
    expect(root.style.getPropertyValue('--a-9')).toBe('');
  });

  it('resolves system theme from the OS and follows changes', () => {
    controller = new ThemeController(root);
    controller.apply({ ...basePrefs, theme: 'system' });
    expect(root.dataset.theme).toBe('light');
    systemDark = true;
    mediaListeners.forEach((listener) => listener());
    expect(root.dataset.theme).toBe('dark');
    expect(document.querySelector('meta[name="theme-color"]')?.getAttribute('content')).toBe('#090d0d');
  });

  it('keeps an explicit theme when the OS changes', () => {
    controller = new ThemeController(root);
    controller.apply({ ...basePrefs, theme: 'light' });
    systemDark = true;
    mediaListeners.forEach((listener) => listener());
    expect(root.dataset.theme).toBe('light');
  });

  it('marks the switching guard when the resolved theme changes', () => {
    controller = new ThemeController(root);
    controller.apply({ ...basePrefs, theme: 'light' });
    controller.apply({ ...basePrefs, theme: 'dark' });
    expect(root.hasAttribute('data-theme-switching')).toBe(true);
  });

  it('removes the OS listener on dispose', () => {
    controller = new ThemeController(root);
    controller.apply({ ...basePrefs, theme: 'system' });
    controller.dispose();
    expect(mediaListeners.size).toBe(0);
    controller = null;
  });
});
