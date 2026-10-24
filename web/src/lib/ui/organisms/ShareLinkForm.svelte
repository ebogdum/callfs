<script lang="ts" module>
	export interface ShareLinkResult {
		url: string;
		expiresAt: string;
	}

	export const TOKENS = ['--share-form-gap', '--share-facts-gap', '--share-facts-fg'] as const;

	const PRESET_SECONDS: number[] = [900, 3600, 86400];
	const RECOMMENDED_SECONDS = 3600;
	const CUSTOM = 'custom';
	const MS_PER_SECOND = 1000;
	const COUNTDOWN_TICK_MS = 1000;

	const PRESET_COPY: Record<string, string> = {
		'900': 'fileforms.share.preset.900',
		'3600': 'fileforms.share.preset.3600',
		'86400': 'fileforms.share.preset.86400'
	};

	function choiceFor(seconds: number): string {
		return PRESET_SECONDS.includes(seconds) ? String(seconds) : CUSTOM;
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { formatBytes } from '$lib/format/bytes';
	import { formatDuration, parseGoDuration, toGoDuration } from '$lib/format/duration';
	import { baseName } from '$lib/format/path';
	import { formatAbsolute } from '$lib/format/time';
	import Button from '../atoms/Button.svelte';
	import CopyButton from '../atoms/CopyButton.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import CopyField from '../molecules/CopyField.svelte';
	import DurationInput from '../molecules/DurationInput.svelte';
	import InlineAlert from '../molecules/InlineAlert.svelte';
	import RadioGroup from '../molecules/RadioGroup.svelte';
	import type { ChoiceOption } from '../molecules/RadioGroup.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLElement>, 'class'> {
		path: string;
		size: number | null;
		maxSeconds?: number;
		value?: number;
		result: ShareLinkResult | null;
		busy?: boolean;
		ongenerate: () => void;
		class?: string;
	}

	let {
		path,
		size,
		maxSeconds = 86400,
		value = $bindable(RECOMMENDED_SECONDS),
		result,
		busy = false,
		ongenerate,
		class: className,
		...rest
	}: Props = $props();

	const uid = $props.id();
	const targetId = `${uid}-target`;
	const countdownId = `${uid}-countdown`;

	const isFolder = $derived('/' === path.charAt(path.length - 1));
	const name = $derived(baseName(path));
	const target = $derived(t('fileforms.share.target', { name, size: formatBytes(size).text }));

	const options = $derived<ChoiceOption[]>([
		...PRESET_SECONDS.map((seconds) => ({
			value: String(seconds),
			label: t(PRESET_COPY[String(seconds)]),
			recommended: RECOMMENDED_SECONDS === seconds
		})),
		{ value: CUSTOM, label: t('fileforms.share.preset.custom') }
	]);

	let choice = $state(choiceFor(value));
	let customText = $state(toGoDuration(value * MS_PER_SECOND) ?? '1h');
	const maxText = $derived(toGoDuration(maxSeconds * MS_PER_SECOND) ?? '');

	$effect(() => {
		if (CUSTOM === choice) {
			const parsed = parseGoDuration(customText);
			if (parsed.ok) {
				value = parsed.value / MS_PER_SECOND;
			}
			return;
		}
		value = Number(choice);
	});

	const expiresLabel = $derived(formatAbsolute(Date.now() + value * MS_PER_SECOND));
	const facts = $derived([
		t('fileforms.share.fact_once'),
		t('fileforms.share.fact_anyone', { name }),
		t('fileforms.share.fact_expires', { date: expiresLabel }),
		t('fileforms.share.fact_opening')
	]);

	let now = $state(Date.now());

	$effect(() => {
		if (null === result) {
			return;
		}
		const timer = setInterval(() => {
			now = Date.now();
		}, COUNTDOWN_TICK_MS);
		return () => clearInterval(timer);
	});

	const expiresMs = $derived(null === result ? Number.NaN : Date.parse(result.expiresAt));
	const remainingMs = $derived(Number.isNaN(expiresMs) ? 0 : Math.max(0, expiresMs - now));
	const expired = $derived(0 === remainingMs);
	const curl = $derived(null === result ? '' : `curl -fOJ "${result.url}"`);
</script>

<section class={['share-link-form', className]} aria-label={t('fileforms.share.region')} {...rest}>
	<Stack gap="4">
		<HelpText id={targetId}>{target}</HelpText>
		{#if isFolder}
			<InlineAlert tone="warn" title={t('fileforms.share.folder_blocked')} />
		{/if}
		<RadioGroup bind:value={choice} {options} label={t('fileforms.share.expiry')} />
		{#if CUSTOM === choice}
			<DurationInput bind:value={customText} label={t('fileforms.share.custom')} min="1s" max={maxText} />
		{/if}
		<ul class="facts" aria-label={t('fileforms.share.facts')}>
			{#each facts as fact (fact)}
				<li>{fact}</li>
			{/each}
		</ul>
		<Cluster gap="2">
			<Button variant="primary" loading={busy} disabledReason={isFolder ? t('fileforms.share.folder_blocked') : undefined} onclick={ongenerate}>
				{t('fileforms.share.generate')}
			</Button>
		</Cluster>
		{#if null !== result}
			<Stack gap="3">
				<CopyField label={t('fileforms.share.url')} value={result.url} mono />
				<Cluster gap="2">
					<CopyButton value={curl} label={t('fileforms.share.copy_curl')} variant="button" />
					<HelpText id={countdownId}>
						{expired ? t('fileforms.share.expired') : t('fileforms.share.countdown', { time: formatDuration(remainingMs, 'short') })}
					</HelpText>
				</Cluster>
			</Stack>
		{/if}
	</Stack>
</section>

<style>
	.share-link-form {
		display: grid;
		min-inline-size: 0;
	}

	.facts {
		display: grid;
		gap: var(--share-facts-gap, var(--space-1));
		margin: 0;
		padding: 0;
		color: var(--share-facts-fg, var(--text-muted));
		font-size: var(--text-sm-size);
		line-height: var(--text-sm-line);
		list-style: none;
	}
</style>
