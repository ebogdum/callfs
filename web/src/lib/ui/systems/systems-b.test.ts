import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import { describe, expect, it, vi } from 'vitest';
import type { Command, CommandContext } from '$lib/commands/registry.svelte';
import { t } from '$lib/copy/t';
import copy from '$lib/copy/en/systems-b';
import { COMMANDS_CONTEXT } from '../planets/cluster/MembersPlanet.svelte';
import ClusterSystem from './ClusterSystem.svelte';
import SharingSystem from './SharingSystem.svelte';

const CONTEXT: CommandContext = { role: 'member', route: '/sharing', selectionCount: 0, capabilities: [] };

function childSnippet(text: string) {
	return createRawSnippet(() => ({ render: () => `<p>${text}</p>` }));
}

describe('systems-b copy', () => {
	it('keys all carry the systems-b prefix', () => {
		expect(Object.keys(copy).every((key) => key.startsWith('systems-b.'))).toBe(true);
	});

	it('resolves the sharing command title through t()', () => {
		expect(t('systems-b.sharing.command.title')).toBe('New download link');
	});
});

describe('SharingSystem', () => {
	it('renders its route children inside the system root', () => {
		render(SharingSystem, { props: { children: childSnippet('Sharing page body') } });

		expect(screen.getByText('Sharing page body')).toBeTruthy();
	});

	it('registers the new download link command in the Sharing group', () => {
		const register = vi.fn<(commands: readonly Command[]) => () => void>(() => () => undefined);

		render(SharingSystem, {
			props: { children: childSnippet('Sharing page body') },
			context: new Map([[COMMANDS_CONTEXT, { register }]])
		});

		expect(register).toHaveBeenCalledTimes(1);
		const [commands] = register.mock.calls[0];
		const command = commands.find((entry) => 'sharing.link.new' === entry.id);
		expect(command?.group).toBe('Sharing');
		expect(command?.title).toBe(t('systems-b.sharing.command.title'));
	});

	it('opens the file path dialog when the command runs', async () => {
		const register = vi.fn<(commands: readonly Command[]) => () => void>(() => () => undefined);

		render(SharingSystem, {
			props: { children: childSnippet('Sharing page body') },
			context: new Map([[COMMANDS_CONTEXT, { register }]])
		});
		const [commands] = register.mock.calls[0];
		const command = commands.find((entry) => 'sharing.link.new' === entry.id);
		command?.run(CONTEXT);

		expect(await screen.findByRole('dialog', { name: t('systems-b.sharing.dialog.title') })).toBeTruthy();
		expect(screen.getByLabelText(t('systems-b.sharing.dialog.path'))).toBeTruthy();
	});
});

describe('ClusterSystem', () => {
	it('shows the admin-only empty state and hides cluster children for members', () => {
		render(ClusterSystem, { props: { children: childSnippet('Cluster page body') } });

		expect(screen.getByText(t('systems-b.cluster.admin_only.title'))).toBeTruthy();
		expect(screen.queryByText('Cluster page body')).toBeNull();
	});
});
