<script lang="ts" module>
	let instanceCount = 0;
</script>

<script lang="ts">
	import type { HTMLAttributes } from 'svelte/elements';
	import { t } from '$lib/copy/t';
	import { isValidMode, modeToSymbolic, symbolicToMode } from '$lib/format/mode';
	import Checkbox from '../atoms/Checkbox.svelte';
	import FieldError from '../atoms/FieldError.svelte';
	import Input from '../atoms/Input.svelte';
	import Label from '../atoms/Label.svelte';
	import Text from '../atoms/Text.svelte';
	import Stack from '../quarks/Stack.svelte';

	interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'children'> {
		value: string;
		label: string;
	}

	let { value = $bindable(), label, class: className, ...rest }: Props = $props();

	const idPrefix = `octal-mode-${++instanceCount}`;
	const inputId = `${idPrefix}-input`;
	const errorId = `${idPrefix}-error`;

	const ROLES = [
		{ id: 'owner', name: t('editors.octal.owner') },
		{ id: 'group', name: t('editors.octal.group') },
		{ id: 'others', name: t('editors.octal.others') }
	];
	const PERMISSIONS = [
		{ name: t('editors.octal.read'), weight: 4 },
		{ name: t('editors.octal.write'), weight: 2 },
		{ name: t('editors.octal.run'), weight: 1 }
	];

	const valid = $derived(isValidMode(value));
	const digits = $derived(valid ? Array.from(value.slice(-3), (char) => Number(char)) : [0, 0, 0]);
	const symbolic = $derived(modeToSymbolic(value));
	let typed = $state<string | null>(null);
	const symbolicInputId = `${idPrefix}-symbolic`;
	const symbolicErrorId = `${idPrefix}-symbolic-error`;
	const symbolicShown = $derived(typed ?? (symbolic.ok ? symbolic.value : ''));
	const symbolicInvalid = $derived(null !== typed && !symbolicToMode(typed, valid ? value : '000').ok);

	function setSymbolic(next: string): void {
		typed = next;
		const parsed = symbolicToMode(next, valid ? value : '000');
		if (parsed.ok) {
			const prefix = valid && 4 === value.length ? value.charAt(0) : '';
			value = `${prefix}${parsed.value.slice(1)}`;
		}
	}

	function isChecked(roleIndex: number, weight: number): boolean {
		return 0 !== (digits[roleIndex] & weight);
	}

	function setPermission(roleIndex: number, weight: number, granted: boolean): void {
		const updatedDigits = digits.map((digit, index) => {
			if (roleIndex !== index) {
				return digit;
			}
			return granted ? digit | weight : digit & ~weight;
		});
		const prefix = valid && 4 === value.length ? value.charAt(0) : '';
		value = `${prefix}${updatedDigits.join('')}`;
	}
</script>

<div class={['octal-mode-editor', className]} data-state={valid ? 'valid' : 'invalid'} {...rest}>
	<fieldset>
		<legend>
			<Text size="caption" tone="muted">{label}</Text>
		</legend>
		<div class="permission-grid">
			{#each ROLES as role, roleIndex (role.id)}
				<div class="permission-row" role="group" aria-label={role.name}>
					<Text>{role.name}</Text>
					{#each PERMISSIONS as permission (permission.weight)}
						<Checkbox
							label={permission.name}
							bind:checked={
								() => isChecked(roleIndex, permission.weight),
								(granted) => setPermission(roleIndex, permission.weight, granted)
							}
						/>
					{/each}
				</div>
			{/each}
		</div>
		<Stack gap="1-5">
			<Label for={inputId}>{t('editors.octal.input_label')}</Label>
			<Input
				id={inputId}
				mono
				inputmode="numeric"
				maxlength={4}
				autocomplete="off"
				invalid={!valid}
				aria-describedby={!valid ? errorId : undefined}
				bind:value={() => value, (next) => (value = next)}
			/>
			{#if false === valid}
				<FieldError id={errorId} message={t('editors.octal.invalid')} />
			{/if}
			<Label for={symbolicInputId}>{t('editors.octal.symbolic_label')}</Label>
			<Input
				id={symbolicInputId}
				mono
				autocomplete="off"
				invalid={symbolicInvalid}
				aria-describedby={symbolicInvalid ? symbolicErrorId : undefined}
				onblur={() => (typed = null)}
				bind:value={() => symbolicShown, setSymbolic}
			/>
			{#if symbolicInvalid}
				<FieldError id={symbolicErrorId} message={t('editors.octal.symbolic_invalid')} />
			{/if}
		</Stack>
	</fieldset>
</div>

<style>
	fieldset {
		min-inline-size: 0;
		border: 0;
		padding: 0;
	}

	legend {
		padding: 0;
	}

	.permission-grid {
		display: grid;
		grid-template-columns: max-content repeat(3, max-content);
		align-items: center;
		gap: var(--octal-mode-editor-gap, var(--space-2) var(--space-4));
	}

	.permission-row {
		display: contents;
	}
</style>
