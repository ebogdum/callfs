export type Result<T> = { ok: true; value: T } | { ok: false; error: string };

export type Rgb = { r: number; g: number; b: number };

const HEX_PATTERN = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i;

export function hexToRgb(hex: string): Result<Rgb> {
	const digits = HEX_PATTERN.exec(hex)?.[1];
	if (undefined === digits) {
		return { ok: false, error: `Not a hex color: ${hex}` };
	}
	const full = 3 === digits.length ? digits.replace(/./g, '$&$&') : digits;
	return {
		ok: true,
		value: {
			r: Number.parseInt(full.slice(0, 2), 16),
			g: Number.parseInt(full.slice(2, 4), 16),
			b: Number.parseInt(full.slice(4, 6), 16)
		}
	};
}

function linearChannel(channel: number): number {
	const srgb = channel / 255;
	return 0.04045 >= srgb ? srgb / 12.92 : ((srgb + 0.055) / 1.055) ** 2.4;
}

export function relativeLuminance({ r, g, b }: Rgb): number {
	return 0.2126 * linearChannel(r) + 0.7152 * linearChannel(g) + 0.0722 * linearChannel(b);
}

export function contrastRatio(a: string, b: string): Result<number> {
	const first = hexToRgb(a);
	if (false === first.ok) {
		return first;
	}
	const second = hexToRgb(b);
	if (false === second.ok) {
		return second;
	}
	const firstLuminance = relativeLuminance(first.value);
	const secondLuminance = relativeLuminance(second.value);
	const lighter = Math.max(firstLuminance, secondLuminance);
	const darker = Math.min(firstLuminance, secondLuminance);
	return { ok: true, value: (lighter + 0.05) / (darker + 0.05) };
}

function encodeChannel(linear: number): string {
	const clamped = Math.min(1, Math.max(0, linear));
	const encoded = 0.0031308 >= clamped ? 12.92 * clamped : 1.055 * clamped ** (1 / 2.4) - 0.055;
	return Math.round(encoded * 255)
		.toString(16)
		.padStart(2, '0');
}

export function oklchToHex(l: number, c: number, h: number): string {
	const hue = (h * Math.PI) / 180;
	const a = c * Math.cos(hue);
	const b = c * Math.sin(hue);
	const longCone = (l + 0.3963377774 * a + 0.2158037573 * b) ** 3;
	const mediumCone = (l - 0.1055613458 * a - 0.0638541728 * b) ** 3;
	const shortCone = (l - 0.0894841775 * a - 1.291485548 * b) ** 3;
	const red = 4.0767416621 * longCone - 3.3077115913 * mediumCone + 0.2309699292 * shortCone;
	const green = -1.2684380046 * longCone + 2.6097574011 * mediumCone - 0.3413193965 * shortCone;
	const blue = -0.0041960863 * longCone - 0.7034186147 * mediumCone + 1.707614701 * shortCone;
	return `#${encodeChannel(red)}${encodeChannel(green)}${encodeChannel(blue)}`;
}
