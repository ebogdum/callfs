import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { createRawSnippet, tick } from 'svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { t } from '$lib/copy/t';
import CodeView, { tokenizeLine } from './CodeView.svelte';
import ConfigDiff from './ConfigDiff.svelte';
import SettingRow, { type SettingMeta } from './SettingRow.svelte';
import SettingsSection from './SettingsSection.svelte';

afterEach(() => {
	cleanup();
});

const meta = (overrides: Partial<SettingMeta> = {}): SettingMeta => ({
	key: 'server.read_timeout',
	type: 'duration',
	label: 'Time to receive a request',
	effect: 'Uploads that take longer than this stop.',
	restart: 'restart',
	source: 'file',
	valueText: '30 seconds',
	defaultValue: '30s',
	defaultText: '30 seconds',
	...overrides,
});

const rowProps = (overrides: Record<string, unknown> = {}) => ({
	meta: meta(),
	value: '30s',
	draft: '30s',
	status: 'clean' as const,
	...overrides,
});

describe('SettingRow', () => {
	it('renders a switch named by the setting label for bool settings', () => {
		render(SettingRow, {
			props: rowProps({
				meta: meta({ type: 'bool', label: 'Allow HTTP/3 (QUIC)', effect: 'Needs a certificate.', defaultValue: false, defaultText: 'Off', valueText: 'Off' }),
				value: false,
				draft: false,
			}),
		});
		expect(screen.getByRole('switch', { name: /Allow HTTP\/3 \(QUIC\)/ })).toBeTruthy();
	});

	it('renders one radio per option for enum settings', () => {
		render(SettingRow, {
			props: rowProps({
				meta: meta({
					type: 'enum',
					label: 'Connection type',
					options: [
						{ value: 'https', label: 'HTTPS' },
						{ value: 'http', label: 'HTTP' },
					],
					defaultValue: 'https',
					defaultText: 'HTTPS',
					valueText: 'HTTPS',
				}),
				value: 'https',
				draft: 'https',
			}),
		});
		expect(screen.getAllByRole('radio')).toHaveLength(2);
	});

	it('labels a number editor with the setting label', () => {
		render(SettingRow, {
			props: rowProps({
				meta: meta({ type: 'number', label: 'Download link opens per second', min: 1, defaultValue: 10, defaultText: '10', valueText: '10' }),
				value: 10,
				draft: 10,
			}),
		});
		expect(screen.getByLabelText('Download link opens per second')).toBeTruthy();
	});

	it('labels a text editor with the setting label', () => {
		render(SettingRow, {
			props: rowProps({
				meta: meta({ type: 'text', label: 'Listen address', defaultValue: ':8443', defaultText: ':8443', valueText: ':8443' }),
				value: ':8443',
				draft: ':8443',
			}),
		});
		expect(screen.getByLabelText('Listen address')).toBeTruthy();
	});

	it('shows the environment lock as read-only text without an editor', () => {
		render(SettingRow, {
			props: rowProps({ meta: meta({ source: 'env' }) }),
		});
		expect(screen.getByText(t('settingsui.env.locked', { name: 'CALLFS_SERVER__READ_TIMEOUT' }))).toBeTruthy();
		expect(screen.queryByRole('textbox')).toBeNull();
		expect(screen.queryByRole('button', { name: t('settingsui.reset') })).toBeNull();
	});

	it('shows the restart badge and source badge', () => {
		const { unmount } = render(SettingRow, {
			props: rowProps({ meta: meta({ restart: 'runtime', source: 'default' }) }),
		});
		expect(screen.getByText(t('settingsui.restart.applies'))).toBeTruthy();
		expect(screen.getByText(t('settingsui.source.default'))).toBeTruthy();
		unmount();
		render(SettingRow, {
			props: rowProps({ meta: meta({ restart: 'restart' }) }),
		});
		expect(screen.getByText(t('settingsui.restart.needed'))).toBeTruthy();
	});

	it('calls onreset when the draft differs from the default', async () => {
		const onreset = vi.fn();
		render(SettingRow, {
			props: rowProps({ draft: '2m', status: 'dirty', onreset }),
		});
		await fireEvent.click(screen.getByRole('button', { name: t('settingsui.reset') }));
		expect(onreset).toHaveBeenCalledTimes(1);
	});

	it('hides the reset button when the draft equals the default', () => {
		render(SettingRow, { props: rowProps() });
		expect(screen.queryByRole('button', { name: t('settingsui.reset') })).toBeNull();
	});

	it('marks the row as changed when the draft differs from the saved value', () => {
		render(SettingRow, {
			props: rowProps({ draft: '1m', status: 'dirty' }),
		});
		expect(screen.getByText(t('settingsui.status.changed'))).toBeTruthy();
	});

	it('reveals the technical key only when technical details are on', () => {
		const hidden = render(SettingRow, { props: rowProps() });
		expect(hidden.queryByText('server.read_timeout')).toBeNull();
		expect(hidden.queryByRole('button', { name: t('settingsui.technical.copy_key') })).toBeNull();
		hidden.unmount();
		const shown = render(SettingRow, { props: rowProps({ technical: true }) });
		expect(shown.getByText('server.read_timeout')).toBeTruthy();
		expect(shown.getByRole('button', { name: t('settingsui.technical.copy_key') })).toBeTruthy();
	});

	it('shows the validation error text when the status is invalid', () => {
		render(SettingRow, {
			props: rowProps({ draft: 'soon', status: 'invalid', error: 'Enter a duration such as 30s.' }),
		});
		expect(screen.getByText('Enter a duration such as 30s.')).toBeTruthy();
	});
});

const sectionRows = createRawSnippet(() => ({ render: () => '<p>Row body</p>' }));

describe('SettingsSection', () => {
	it('renders the title as a level 2 heading and the rows', () => {
		render(SettingsSection, {
			props: { title: 'Timeouts', rows: sectionRows, mode: 'autosave', dirtyCount: 0 },
		});
		expect(screen.getByRole('heading', { level: 2, name: 'Timeouts' })).toBeTruthy();
		expect(screen.getByText('Row body')).toBeTruthy();
	});

	it('shows the save bar with the change count and calls onsave', async () => {
		const onsave = vi.fn();
		render(SettingsSection, {
			props: { title: 'Timeouts', rows: sectionRows, mode: 'transactional', dirtyCount: 3, onsave },
		});
		expect(screen.getByText(t('settingsui.save.count', { count: 3 }))).toBeTruthy();
		await fireEvent.click(screen.getByRole('button', { name: t('settingsui.save.review') }));
		expect(onsave).toHaveBeenCalledTimes(1);
	});

	it('hides the save bar when nothing is dirty', () => {
		render(SettingsSection, {
			props: { title: 'Timeouts', rows: sectionRows, mode: 'transactional', dirtyCount: 0 },
		});
		expect(screen.queryByRole('button', { name: t('settingsui.save.review') })).toBeNull();
	});

	it('shows Saved with Undo for autosave sections and calls onundo', async () => {
		const onundo = vi.fn();
		render(SettingsSection, {
			props: { title: 'Console', rows: sectionRows, mode: 'autosave', dirtyCount: 0, saved: true, onundo },
		});
		await tick();
		expect(screen.getByText(t('settingsui.autosave.saved'))).toBeTruthy();
		await fireEvent.click(screen.getByRole('button', { name: t('settingsui.autosave.undo') }));
		expect(onundo).toHaveBeenCalledTimes(1);
	});

	it('lists presets with Custom last and calls onapplypreset with the chosen id', async () => {
		const onapplypreset = vi.fn();
		render(SettingsSection, {
			props: {
				title: 'Limits',
				rows: sectionRows,
				mode: 'autosave',
				dirtyCount: 0,
				presets: [
					{ id: 'standard', label: 'Standard', recommended: true },
					{ id: 'large', label: 'Large files' },
				],
				onapplypreset,
			},
		});
		const radios = screen.getAllByRole('radio');
		expect(radios).toHaveLength(3);
		expect(screen.getByRole('radio', { name: t('settingsui.presets.custom') })).toBeTruthy();
		await fireEvent.click(screen.getByRole('radio', { name: /Large files/ }));
		expect(onapplypreset).toHaveBeenCalledWith('large');
	});
});

describe('ConfigDiff', () => {
	it('shows the no-changes message when before and after match', () => {
		render(ConfigDiff, {
			props: { before: { 'server.listen_addr': ':8443' }, after: { 'server.listen_addr': ':8443' }, secretKeys: [] },
		});
		expect(screen.getByText(t('settingsui.diff.none'))).toBeTruthy();
	});

	it('describes a changed key from and to with its restart effect', () => {
		render(ConfigDiff, {
			props: {
				before: { 'server.read_timeout': '30s' },
				after: { 'server.read_timeout': '2m' },
				secretKeys: [],
				restartKeys: ['server.read_timeout'],
			},
		});
		expect(screen.getByText('server.read_timeout')).toBeTruthy();
		expect(screen.getByText(t('settingsui.diff.changed', { from: '30s', to: '2m' }))).toBeTruthy();
		expect(screen.getByText(t('settingsui.restart.needed'))).toBeTruthy();
	});

	it('shows secret changes as Replaced without the values', () => {
		render(ConfigDiff, {
			props: {
				before: { 'auth.internal_proxy_secret': 'old-secret-value' },
				after: { 'auth.internal_proxy_secret': 'new-secret-value' },
				secretKeys: ['auth.internal_proxy_secret'],
			},
		});
		expect(screen.getByText(t('settingsui.diff.replaced'))).toBeTruthy();
		expect(screen.queryByText(/old-secret-value/)).toBeNull();
		expect(screen.queryByText(/new-secret-value/)).toBeNull();
	});
});

describe('CodeView', () => {
	it('tokenizes yaml keys, numbers and comments', () => {
		const tokens = tokenizeLine('read_timeout: 30s # slow', 'yaml');
		expect(tokens).toContainEqual({ kind: 'key', text: 'read_timeout' });
		expect(tokens).toContainEqual({ kind: 'number', text: '30s' });
		expect(tokens).toContainEqual({ kind: 'comment', text: '# slow' });
	});

	it('tokenizes json keys, strings and punctuation', () => {
		const tokens = tokenizeLine('"port": 8443,', 'json');
		expect(tokens).toContainEqual({ kind: 'key', text: '"port"' });
		expect(tokens).toContainEqual({ kind: 'number', text: '8443' });
		expect(tokens).toContainEqual({ kind: 'punct', text: ',' });
	});

	it('tokenizes env names and values', () => {
		const tokens = tokenizeLine('CALLFS_PORT=8443', 'env');
		expect(tokens).toContainEqual({ kind: 'key', text: 'CALLFS_PORT' });
		expect(tokens).toContainEqual({ kind: 'punct', text: '=' });
	});

	it('tokenizes shell options and variables', () => {
		const tokens = tokenizeLine('curl --insecure $TOKEN', 'shell');
		expect(tokens).toContainEqual({ kind: 'option', text: '--insecure' });
		expect(tokens).toContainEqual({ kind: 'variable', text: '$TOKEN' });
	});

	it('tokenizes http methods and header names', () => {
		expect(tokenizeLine('GET /v1/files HTTP/1.1', 'http')).toContainEqual({ kind: 'keyword', text: 'GET' });
		expect(tokenizeLine('Host: example.com', 'http')).toContainEqual({ kind: 'key', text: 'Host' });
	});

	it('returns the whole line as plain text when nothing matches', () => {
		expect(tokenizeLine('plain words', 'shell')).toEqual([{ kind: 'plain', text: 'plain words' }]);
	});

	it('renders a copy button and line numbers when requested', () => {
		const { container } = render(CodeView, {
			props: { code: 'a: 1\nb: 2', language: 'yaml', label: 'Config YAML', lineNumbers: true },
		});
		expect(screen.getByRole('button', { name: t('settingsui.code.copy') })).toBeTruthy();
		const gutters = Array.from(container.querySelectorAll('.gutter')).map((node) => node.textContent);
		expect(gutters).toEqual(['1', '2']);
	});
});
