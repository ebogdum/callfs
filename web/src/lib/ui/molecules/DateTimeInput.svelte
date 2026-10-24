<script lang="ts" module>
	const TEXT_PATTERN = /^(\d{4})-(\d{2})-(\d{2}) (\d{2}):(\d{2})$/;
	const DAY_STEPS: Record<string, number> = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7 };
	const MONTH_STEPS: Record<string, number> = { PageUp: -1, PageDown: 1 };
	const WEEKDAY_KEYS = [
		'u076molecules.weekday_0',
		'u076molecules.weekday_1',
		'u076molecules.weekday_2',
		'u076molecules.weekday_3',
		'u076molecules.weekday_4',
		'u076molecules.weekday_5',
		'u076molecules.weekday_6'
	];

	function twoDigits(part: number): string {
		return String(part).padStart(2, '0');
	}

	function toText(time: number): string {
		const date = new Date(time);
		return `${date.getFullYear()}-${twoDigits(date.getMonth() + 1)}-${twoDigits(date.getDate())} ${twoDigits(date.getHours())}:${twoDigits(date.getMinutes())}`;
	}

	function parseText(text: string): number | null {
		const match = TEXT_PATTERN.exec(text);
		if (null === match) {
			return null;
		}
		const [year, month, day, hour, minute] = match.slice(1).map(Number);
		const parsed = new Date(year, month - 1, day, hour, minute);
		const isExact =
			parsed.getFullYear() === year &&
			parsed.getMonth() === month - 1 &&
			parsed.getDate() === day &&
			23 >= hour &&
			59 >= minute;
		return isExact ? parsed.getTime() : null;
	}

	function buildDays(anchor: Date): Date[] {
		const year = anchor.getFullYear();
		const month = anchor.getMonth();
		const offset = new Date(year, month, 1).getDay();
		return Array.from({ length: 42 }, (_, index) => new Date(year, month, 1 - offset + index));
	}

	function sameDay(first: Date, second: Date): boolean {
		return first.toDateString() === second.toDateString();
	}

	function shiftDay(day: Date, key: string): Date | null {
		if (key in DAY_STEPS) {
			return new Date(day.getFullYear(), day.getMonth(), day.getDate() + DAY_STEPS[key]);
		}
		if (key in MONTH_STEPS) {
			return new Date(day.getFullYear(), day.getMonth() + MONTH_STEPS[key], 1);
		}
		return null;
	}
</script>

<script lang="ts">
	import { tick } from 'svelte';
	import type { HTMLAttributes } from 'svelte/elements';
	import { Popover } from 'bits-ui';
	import Button from '../atoms/Button.svelte';
	import FieldError from '../atoms/FieldError.svelte';
	import HelpText from '../atoms/HelpText.svelte';
	import Input from '../atoms/Input.svelte';
	import Label from '../atoms/Label.svelte';
	import { formatAbsolute, formatTimestamp } from '$lib/format/time';
	import { t } from '$lib/copy/t';

	interface Props extends HTMLAttributes<HTMLDivElement> {
		value?: number;
		label: string;
	}

	let { value = $bindable(0), label, class: className, ...rest }: Props = $props();

	const uid = $props.id();
	const inputId = `${uid}-input`;
	const helpId = `${uid}-help`;
	const errorId = `${uid}-error`;

	let text = $state('');
	let invalid = $state(false);
	let open = $state(false);
	let cursor = $state(new Date());
	let grid: HTMLDivElement | undefined = $state();

	const days = $derived(buildDays(cursor));
	const selectedDay = $derived(new Date(value));
	const describedBy = $derived(invalid ? `${helpId} ${errorId}` : helpId);
	const zoneText = $derived(
		t('u076molecules.datetime_local_zone', { time: formatTimestamp(new Date(value)) })
	);

	$effect(() => {
		text = toText(value);
		invalid = false;
	});

	$effect(() => {
		if (open) {
			cursor = new Date(value);
		}
	});

	function commitText() {
		const parsed = parseText(text);
		if (null === parsed) {
			invalid = true;
			return;
		}
		invalid = false;
		value = parsed;
	}

	function handleInputKey(event: KeyboardEvent) {
		if ('Enter' === event.key) {
			event.preventDefault();
			commitText();
		}
	}

	function pickDay(day: Date) {
		const next = new Date(value);
		next.setFullYear(day.getFullYear(), day.getMonth(), day.getDate());
		value = next.getTime();
		open = false;
	}

	function shiftMonth(delta: number) {
		cursor = new Date(cursor.getFullYear(), cursor.getMonth() + delta, 1);
	}

	async function handleGridKey(event: KeyboardEvent) {
		const next = shiftDay(cursor, event.key);
		if (null === next) {
			return;
		}
		event.preventDefault();
		cursor = next;
		await tick();
		grid?.querySelector<HTMLButtonElement>('[tabindex="0"]')?.focus();
	}
</script>

<div class={['date-time', className]} {...rest}>
	<Label for={inputId}>{label}</Label>
	<div class="row">
		<div class="field">
			<Input
				id={inputId}
				bind:value={text}
				mono
				invalid={invalid}
				autocomplete="off"
				enterkeyhint="done"
				placeholder={t('u076molecules.datetime_placeholder')}
				aria-describedby={describedBy}
				onchange={commitText}
				onkeydown={handleInputKey}
			/>
		</div>
		<Popover.Root bind:open>
			<Popover.Trigger>
				{#snippet child({ props })}
					<Button {...props} variant="secondary">{t('u076molecules.datetime_pick')}</Button>
				{/snippet}
			</Popover.Trigger>
			<Popover.Content side="bottom" align="start" sideOffset={4}>
				<div class="calendar" role="dialog" aria-label={t('u076molecules.datetime_calendar', { label })}>
					<div class="calendar-head">
						<Button variant="ghost" size="sm" iconStart="chevron-left" onclick={() => shiftMonth(-1)}>
							{t('u076molecules.datetime_previous_month')}
						</Button>
						<span class="month">{formatAbsolute(new Date(cursor.getFullYear(), cursor.getMonth(), 1))}</span>
						<Button variant="ghost" size="sm" iconEnd="chevron-right" onclick={() => shiftMonth(1)}>
							{t('u076molecules.datetime_next_month')}
						</Button>
					</div>
					<div class="week" aria-hidden="true">
						{#each WEEKDAY_KEYS as key (key)}
							<span>{t(key)}</span>
						{/each}
					</div>
					<div class="days" bind:this={grid} onkeydown={handleGridKey}>
						{#each days as day (day.getTime())}
							<button
								type="button"
								class="day"
								data-outside={day.getMonth() !== cursor.getMonth()}
								tabindex={sameDay(day, cursor) ? 0 : -1}
								aria-label={formatAbsolute(day)}
								aria-pressed={sameDay(day, selectedDay)}
								onclick={() => pickDay(day)}
							>
								{day.getDate()}
							</button>
						{/each}
					</div>
				</div>
			</Popover.Content>
		</Popover.Root>
	</div>
	<HelpText id={helpId}>{zoneText}</HelpText>
	<div aria-live="polite">
		{#if invalid}
			<FieldError id={errorId} message={t('u076molecules.datetime_error')} />
		{/if}
	</div>
</div>

<style>
	.date-time {
		display: flex;
		flex-direction: column;
		gap: var(--space-1-5);
		min-inline-size: 0;
	}

	.row {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: var(--space-2);
	}

	.field {
		flex: 1 1 calc(var(--space-4) * 10);
		min-inline-size: 0;
	}

	.calendar {
		z-index: var(--z-popover-top);
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		min-inline-size: calc(var(--space-4) * 17);
		padding: var(--space-3);
		border: var(--hairline) solid var(--border-subtle);
		border-radius: var(--radius-lg);
		background: var(--surface-overlay);
		box-shadow: var(--shadow-md);
		color: var(--text);
		font-size: var(--text-sm-size);
	}

	.calendar-head {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: var(--space-2);
	}

	.month {
		font-weight: var(--text-body-weight);
		font-variant-numeric: tabular-nums;
	}

	.week,
	.days {
		display: grid;
		grid-template-columns: repeat(7, minmax(0, 1fr));
		gap: var(--space-1);
		text-align: center;
	}

	.week {
		color: var(--text-muted);
		font-size: var(--text-caption-size);
	}

	.day {
		display: inline-grid;
		place-items: center;
		min-inline-size: var(--target-min);
		min-block-size: var(--target-min);
		padding: 0;
		border: 0;
		border-radius: var(--radius-sm);
		background: none;
		color: var(--text);
		font: inherit;
		font-variant-numeric: tabular-nums;
		cursor: pointer;
	}

	.day[data-outside='true'] {
		color: var(--text-muted);
	}

	@media (hover: hover) {
		.day:hover {
			background: var(--surface-hover);
		}
	}

	.day[aria-pressed='true'] {
		background: var(--accent);
		color: var(--text-on-accent);
	}

	@media (pointer: coarse) {
		.day {
			min-inline-size: var(--touch-target);
			min-block-size: var(--touch-target);
		}
	}

	.day:focus-visible {
		outline: var(--focus-width) solid transparent;
		box-shadow: var(--focus-ring);
	}
</style>
