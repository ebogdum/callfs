<script lang="ts" module>
	const DEFAULT_MINIMUM_LENGTH = 16;
	const RECOMMENDED_LENGTH = 32;
	const GENERATED_BYTES = 32;

	let instanceCount = 0;

	function toBase64Url(bytes: Uint8Array): string {
		let binary = '';
		for (const byte of bytes) {
			binary += String.fromCharCode(byte);
		}
		return btoa(binary).replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '');
	}

	function generateSecret(): string {
		return toBase64Url(crypto.getRandomValues(new Uint8Array(GENERATED_BYTES)));
	}
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import type { Threshold } from '../types';
	import Badge from '../atoms/Badge.svelte';
	import Button from '../atoms/Button.svelte';
	import CopyButton from '../atoms/CopyButton.svelte';
	import FieldError from '../atoms/FieldError.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import Input from '../atoms/Input.svelte';
	import Label from '../atoms/Label.svelte';
	import Meter from '../atoms/Meter.svelte';
	import Num from '../atoms/Num.svelte';
	import Text from '../atoms/Text.svelte';
	import ToggleButton from '../atoms/ToggleButton.svelte';
	import Cluster from '../quarks/Cluster.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'children'> {
		label: string;
		isSet: boolean;
		value?: string;
		generate?: boolean;
		minLength?: number;
		autocomplete?: 'current-password' | 'new-password';
		onreplace?: () => void;
	}

	let {
		label,
		isSet,
		value = $bindable(''),
		generate = true,
		minLength = DEFAULT_MINIMUM_LENGTH,
		autocomplete = 'new-password',
		onreplace,
		class: className,
		...rest
	}: Props = $props();

	const inputId = `secret-input-${++instanceCount}`;
	const helpId = `${inputId}-help`;
	const errorId = `${inputId}-error`;

	let replacing = $state(false);
	let revealed = $state(false);
	let generated = $state(false);

	const editing = $derived(!isSet || replacing);
	const tooShort = $derived(0 < value.length && minLength > value.length);
	const statusTone = $derived(isSet ? 'ok' : 'neutral');
	const statusWord = $derived(isSet ? t('editors.secret.status_set') : t('editors.secret.status_unset'));
	const describedBy = $derived(tooShort ? `${helpId} ${errorId}` : helpId);
	const meterThresholds: Threshold[] = $derived([
		{ value: minLength, tone: 'warn', label: t('editors.secret.meter_minimum') }
	]);

	function startReplace(): void {
		replacing = true;
		value = '';
		generated = false;
		onreplace?.();
	}

	function cancelReplace(): void {
		replacing = false;
		revealed = false;
		generated = false;
		value = '';
	}

	function fillGenerated(): void {
		value = generateSecret();
		generated = true;
	}
</script>

<div class={['secret-input', className]} data-state={editing ? 'editing' : 'set'} {...rest}>
	<Stack gap="1-5">
		{#if editing}
			<Cluster gap="2" align="center">
				<Label for={inputId}>{label}</Label>
				<Badge tone={statusTone}>{statusWord}</Badge>
			</Cluster>
			<Cluster gap="2" align="center">
				<Input
					id={inputId}
					bind:value
					type={revealed ? 'text' : 'password'}
					{autocomplete}
					invalid={tooShort}
					aria-describedby={describedBy}
				/>
				<ToggleButton bind:pressed={revealed} label={t('editors.secret.show')} />
				{#if generate}
					<Button onclick={fillGenerated}>{t('editors.secret.generate')}</Button>
				{/if}
				{#if generated && 0 < value.length}
					<CopyButton value={value} label={t('editors.secret.copy')} variant="button" />
				{/if}
				{#if isSet}
					<Button variant="ghost" onclick={cancelReplace}>{t('editors.secret.cancel')}</Button>
				{/if}
			</Cluster>
			<Meter
				value={Math.min(value.length, RECOMMENDED_LENGTH)}
				max={RECOMMENDED_LENGTH}
				label={t('editors.secret.meter_label')}
				thresholds={meterThresholds}
			/>
			<Text size="sm" tone="muted">
				<Num value={value.length} />
				{t('editors.secret.characters')}
			</Text>
			<HelpText id={helpId}>{t('editors.secret.length_help')}</HelpText>
			{#if tooShort}
				<FieldError id={errorId} message={t('editors.secret.too_short', { min: minLength })} />
			{/if}
		{:else}
			<Cluster gap="2" align="center">
				<Text>{label}</Text>
				<Badge tone={statusTone}>{statusWord}</Badge>
				<Button
					variant="secondary"
					size="sm"
					aria-label={t('editors.secret.replace_named', { name: label })}
					onclick={startReplace}>{t('editors.secret.replace')}</Button
				>
			</Cluster>
		{/if}
	</Stack>
</div>

<style>
	.secret-input {
		min-inline-size: 0;
	}
</style>
