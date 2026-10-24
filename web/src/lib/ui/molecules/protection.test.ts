import { cleanup, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it } from 'vitest';
import DurabilityMeter from './DurabilityMeter.svelte';
import ProgressSteps, { type ProgressStep } from './ProgressSteps.svelte';
import QuorumMeter from './QuorumMeter.svelte';

afterEach(cleanup);

const quorumCases: [voters: number, losses: number, state: string, sentence: string][] = [
	[1, 0, 'at_risk', '1 of 1 up. Cannot lose another.'],
	[1, 1, 'lost', '0 of 1 up. Cannot accept changes.'],
	[1, 2, 'lost', '0 of 1 up. Cannot accept changes.'],
	[1, 3, 'lost', '0 of 1 up. Cannot accept changes.'],
	[2, 0, 'at_risk', '2 of 2 up. Cannot lose another.'],
	[2, 1, 'lost', '1 of 2 up. Cannot accept changes.'],
	[2, 2, 'lost', '0 of 2 up. Cannot accept changes.'],
	[2, 3, 'lost', '0 of 2 up. Cannot accept changes.'],
	[3, 0, 'can_lose', '3 of 3 members up. Can lose 1 and keep working.'],
	[3, 1, 'at_risk', '2 of 3 up. Cannot lose another.'],
	[3, 2, 'lost', '1 of 3 up. Cannot accept changes.'],
	[3, 3, 'lost', '0 of 3 up. Cannot accept changes.'],
	[5, 0, 'can_lose', '5 of 5 members up. Can lose 2 and keep working.'],
	[5, 1, 'can_lose', '4 of 5 members up. Can lose 1 and keep working.'],
	[5, 2, 'at_risk', '3 of 5 up. Cannot lose another.'],
	[5, 3, 'lost', '2 of 5 up. Cannot accept changes.']
];

const durabilityCases: [data: number, spare: number, losses: number, state: string, sentence: string][] = [
	[2, 1, 0, 'full', '3 of 3 pieces. Can lose 1 more.'],
	[2, 1, 1, 'critical', '2 of 3 pieces. Cannot lose another.'],
	[2, 1, 2, 'unreadable', '1 of 3. File cannot be read.'],
	[2, 1, 3, 'unreadable', '0 of 3. File cannot be read.'],
	[4, 2, 0, 'full', '6 of 6 pieces. Can lose 2 more.'],
	[4, 2, 1, 'reduced', '5 of 6 pieces. Can lose 1 more.'],
	[4, 2, 2, 'critical', '4 of 6 pieces. Cannot lose another.'],
	[4, 2, 3, 'unreadable', '3 of 6. File cannot be read.']
];

describe('QuorumMeter', () => {
	it.each(quorumCases)('%i voters with %i losses reads %s', (voters, losses, state, sentence) => {
		const { container } = render(QuorumMeter, { voters, healthy: voters - losses });
		const root = container.firstElementChild;
		expect(root?.getAttribute('data-state')).toBe(state);
		expect(screen.getByText(sentence).tagName).toBe('P');
	});

	it('renders one segment per voter with up and down states', () => {
		const { container } = render(QuorumMeter, { voters: 3, healthy: 2 });
		const members = [...container.querySelectorAll('.member')];
		expect(members.map((member) => member.getAttribute('data-up'))).toEqual(['true', 'true', 'false']);
	});

	it('clamps healthy members into the voter range', () => {
		const { container } = render(QuorumMeter, { voters: 2, healthy: 9 });
		expect(container.querySelectorAll('.member')).toHaveLength(2);
		expect(screen.getByText('2 of 2 up. Cannot lose another.')).toBeTruthy();
	});
});

describe('DurabilityMeter', () => {
	it.each(durabilityCases)('%i data and %i spare with %i losses reads %s', (data, spare, losses, state, sentence) => {
		const { container } = render(DurabilityMeter, { data, spare, healthy: data + spare - losses });
		expect(container.firstElementChild?.getAttribute('data-state')).toBe(state);
		expect(screen.getByText(sentence).tagName).toBe('P');
		expect(container.querySelectorAll('.piece')).toHaveLength(data + spare);
	});

	it('marks data and spare pieces by kind', () => {
		const { container } = render(DurabilityMeter, { data: 4, spare: 2, healthy: 5 });
		const kinds = [...container.querySelectorAll('.piece')].map((piece) => piece.getAttribute('data-kind'));
		expect(kinds).toEqual(['data', 'data', 'data', 'data', 'spare', 'spare']);
		const up = [...container.querySelectorAll('.piece')].filter((piece) => 'true' === piece.getAttribute('data-up'));
		expect(up).toHaveLength(5);
	});
});

describe('ProgressSteps', () => {
	const steps: ProgressStep[] = [
		{ id: 'copy', label: 'Copy', state: 'job.succeeded' },
		{ id: 'check', label: 'Check', state: 'job.running' },
		{ id: 'remove', label: 'Remove source', state: 'job.queued' }
	];

	it('renders every step with its catalog word and marks the running step', () => {
		const { container } = render(ProgressSteps, { steps });
		const items = [...container.querySelectorAll('li')];
		expect(items.map((item) => item.getAttribute('data-state'))).toEqual(['job.succeeded', 'job.running', 'job.queued']);
		expect(items[1].getAttribute('aria-current')).toBe('step');
		expect(items[0].hasAttribute('aria-current')).toBe(false);
		expect(screen.getByText('Done')).toBeTruthy();
		expect(screen.getByText('Running')).toBeTruthy();
		expect(screen.getByText('Waiting')).toBeTruthy();
	});
});
