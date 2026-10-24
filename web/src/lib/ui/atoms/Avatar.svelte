<script lang="ts" module>
	export type AvatarSize = 'sm' | 'md' | 'lg';

	export const TOKENS = ['--avatar-bg', '--avatar-fg', '--avatar-radius'] as const;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';

	interface Props extends HTMLAttributes<HTMLSpanElement> {
		name: string;
		size?: AvatarSize;
	}

	let { name, size = 'md', class: className, ...rest }: Props = $props();

	const AVATAR_TONES = ['accent', 'info', 'ok', 'neutral'] as const;

	function hashName(value: string): number {
		let hash = 0;
		for (let index = 0; index < value.length; index += 1) {
			hash = (hash * 31 + value.charCodeAt(index)) >>> 0;
		}
		return hash;
	}

	const tone = $derived(AVATAR_TONES[hashName(name) % AVATAR_TONES.length]);

	const initials = $derived(
		name
			.trim()
			.split(/\s+/)
			.filter((part) => 0 < part.length)
			.slice(0, 2)
			.map((part) => part.charAt(0).toUpperCase())
			.join('')
	);
</script>

<span
	class={['avatar', className]}
	data-tone={tone}
	data-size={size}
	{...rest}
	aria-hidden="true"
>{initials}</span>

<style>
	.avatar {
		--tone-soft: var(--surface-sunken);
		--tone-text: var(--text-muted);
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		user-select: none;
		border-radius: var(--avatar-radius, var(--radius-full));
		background: var(--avatar-bg, var(--tone-soft));
		color: var(--avatar-fg, var(--tone-text));
		font-weight: 600;
		font-variant-numeric: tabular-nums;
		overflow: hidden;
	}

	.avatar[data-tone='accent'] {
		--tone-soft: var(--accent-soft);
		--tone-text: var(--accent-text);
	}

	.avatar[data-tone='info'] {
		--tone-soft: var(--info-soft);
		--tone-text: var(--info-text);
	}

	.avatar[data-tone='ok'] {
		--tone-soft: var(--ok-soft);
		--tone-text: var(--ok-text);
	}

	.avatar[data-size='sm'] {
		inline-size: var(--control-h);
		block-size: var(--control-h);
		font-size: var(--text-caption-size);
		line-height: var(--text-caption-line);
	}

	.avatar[data-size='md'] {
		inline-size: calc(var(--control-h) * 1.25);
		block-size: calc(var(--control-h) * 1.25);
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
	}

	.avatar[data-size='lg'] {
		inline-size: calc(var(--control-h) * 1.5);
		block-size: calc(var(--control-h) * 1.5);
		font-size: var(--text-title-sm-size);
		line-height: var(--text-title-sm-line);
	}
</style>
