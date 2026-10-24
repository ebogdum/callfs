import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Job } from '$lib/api/types';
import { GLOSSARY } from '$lib/copy/glossary';
import { t } from '$lib/copy/t';
import JobList from './JobList.svelte';
import ApiEquivalent from './ApiEquivalent.svelte';
import SessionExpiryDialog from './SessionExpiryDialog.svelte';
import HelpPanel from './HelpPanel.svelte';

afterEach(() => {
	cleanup();
});

const job = (overrides: Partial<Job> = {}): Job => ({
	id: 'job-1',
	kind: 'delete_recursive',
	state: 'running',
	phase: 'delete',
	progress: { done: 120, total: 1204, bytes_done: 0, bytes_total: null, per_second: 4 },
	created_by: 'alice',
	created_at: '2026-10-20T10:00:00Z',
	started_at: '2026-10-20T10:00:01Z',
	finished_at: null,
	error: null,
	failed_items: [],
	failed_count: 0,
	result: { path: '/team/old' },
	...overrides
});

const noop = (): void => {};

describe('JobList', () => {
	it('shows the empty copy when there are no jobs', () => {
		render(JobList, { props: { jobs: [], onopen: noop, oncancel: noop, onretryfailed: noop } });
		expect(screen.getByText(t('support.jobs.empty'))).toBeTruthy();
	});

	it('renders the kind word, target, progress and state of each job', () => {
		const { container } = render(JobList, {
			props: { jobs: [job()], onopen: noop, oncancel: noop, onretryfailed: noop }
		});
		expect(screen.getByText(t('support.jobs.kind.delete_recursive'))).toBeTruthy();
		expect(screen.getByText(`${t('support.jobs.target')}: /team/old`)).toBeTruthy();
		expect(container.querySelector('[data-state="running"]')).not.toBeNull();
	});

	it('passes the job to onopen when Open is pressed', async () => {
		const onopen = vi.fn();
		const fixture = job();
		render(JobList, { props: { jobs: [fixture], onopen, oncancel: noop, onretryfailed: noop } });
		await fireEvent.click(screen.getByRole('button', { name: t('support.jobs.open') }));
		expect(onopen).toHaveBeenCalledWith(fixture);
	});

	it('passes the job to oncancel when a running job is cancelled', async () => {
		const oncancel = vi.fn();
		const fixture = job();
		render(JobList, { props: { jobs: [fixture], onopen: noop, oncancel, onretryfailed: noop } });
		await fireEvent.click(screen.getByRole('button', { name: t('support.jobs.cancel') }));
		expect(oncancel).toHaveBeenCalledWith(fixture);
	});

	it('disables Cancel with the stopping reason while the job is already cancelling', async () => {
		const oncancel = vi.fn();
		render(JobList, {
			props: {
				jobs: [job({ state: 'cancelling' })],
				onopen: noop,
				oncancel,
				onretryfailed: noop
			}
		});
		const cancel = screen.getByRole('button', { name: t('support.jobs.cancel') });
		expect(cancel.getAttribute('aria-disabled')).toBe('true');
		await fireEvent.click(cancel);
		expect(oncancel).not.toHaveBeenCalled();
	});

	it('offers Retry failed with the failed count when a job finished with errors', async () => {
		const onretryfailed = vi.fn();
		const fixture = job({ state: 'partial', failed_count: 2 });
		render(JobList, { props: { jobs: [fixture], onopen: noop, oncancel: noop, onretryfailed } });
		expect(screen.getByText(t('support.jobs.failed_count', { count: 2 }))).toBeTruthy();
		await fireEvent.click(screen.getByRole('button', { name: t('support.jobs.retry_failed') }));
		expect(onretryfailed).toHaveBeenCalledWith(fixture);
	});

	it('hides Cancel and Retry failed for a finished job without failures', () => {
		render(JobList, {
			props: {
				jobs: [job({ state: 'succeeded', finished_at: '2026-10-20T10:05:00Z' })],
				onopen: noop,
				oncancel: noop,
				onretryfailed: noop
			}
		});
		expect(screen.queryByRole('button', { name: t('support.jobs.cancel') })).toBeNull();
		expect(screen.queryByRole('button', { name: t('support.jobs.retry_failed') })).toBeNull();
	});
});

describe('ApiEquivalent', () => {
	const request = {
		method: 'POST',
		url: 'https://files.example.test/v1/links/generate',
		headers: { 'X-Request-Source': 'console' },
		body: { path: '/team/report.pdf', expires_in: 3600 }
	};

	it('collapses the commands until the trigger is pressed', async () => {
		render(ApiEquivalent, { props: { request } });
		const trigger = screen.getByRole('button', { name: t('support.api.summary') });
		expect(trigger.getAttribute('aria-expanded')).toBe('false');
		expect(screen.getByText(/curl -X POST/)).not.toBeVisible();
		await fireEvent.click(trigger);
		expect(trigger.getAttribute('aria-expanded')).toBe('true');
		expect(screen.getByText(/curl -X POST/)).toBeVisible();
		expect(screen.getByText(/curl -X POST 'https:\/\/files\.example\.test\/v1\/links\/generate'/)).toBeTruthy();
	});

	it('uses the access key variable in the bearer header and includes the JSON body', async () => {
		const { container } = render(ApiEquivalent, { props: { request } });
		await fireEvent.click(screen.getByRole('button', { name: t('support.api.summary') }));
		expect(container.textContent).toContain('-H "Authorization: Bearer $CALLFS_KEY"');
		expect(container.textContent).toContain("-H 'X-Request-Source: console'");
		expect(container.textContent).toContain('POST https://files.example.test/v1/links/generate HTTP/1.1');
		expect(container.textContent).toContain('"expires_in": 3600');
		expect(screen.getByText(t('support.api.body_label'))).toBeTruthy();
	});

	it('omits the body section and escapes single quotes when the request has no body', async () => {
		const { container } = render(ApiEquivalent, {
			props: { request: { method: 'GET', url: "/v1/files/it's" } }
		});
		await fireEvent.click(screen.getByRole('button', { name: t('support.api.summary') }));
		expect(container.textContent).toContain("'/v1/files/it'\\''s'");
		expect(screen.queryByText(t('support.api.body_label'))).toBeNull();
	});
});

describe('SessionExpiryDialog', () => {
	it('offers Stay signed in and Sign out for an idle warning', async () => {
		const onstay = vi.fn();
		const onsignout = vi.fn();
		render(SessionExpiryDialog, {
			props: { open: true, reason: 'idle', secondsLeft: 125, onstay, onsignout, onreauth: noop }
		});
		expect(screen.getByRole('alertdialog', { name: t('support.session.idle_title') })).toBeTruthy();
		expect(screen.getByText(t('support.session.countdown'))).toBeTruthy();
		await fireEvent.click(screen.getByRole('button', { name: t('support.session.stay') }));
		expect(onstay).toHaveBeenCalledTimes(1);
		await fireEvent.click(screen.getByRole('button', { name: t('support.session.sign_out') }));
		expect(onsignout).toHaveBeenCalledTimes(1);
	});

	it('shows the expiry title for a session that is about to end', () => {
		render(SessionExpiryDialog, {
			props: { open: true, reason: 'expiry', secondsLeft: 120, onstay: noop, onsignout: noop, onreauth: noop }
		});
		expect(screen.getByRole('alertdialog', { name: t('support.session.expiry_title') })).toBeTruthy();
	});

	it('asks for the access key in place when the session has expired', async () => {
		const onreauth = vi.fn();
		render(SessionExpiryDialog, {
			props: { open: true, reason: 'expired', secondsLeft: 0, onstay: noop, onsignout: noop, onreauth }
		});
		expect(screen.queryByRole('button', { name: t('support.session.stay') })).toBeNull();
		const signIn = screen.getByRole('button', { name: t('support.session.sign_in') });
		expect(signIn.getAttribute('aria-disabled')).toBe('true');
		await fireEvent.click(signIn);
		expect(onreauth).not.toHaveBeenCalled();
		const keyField = screen.getByLabelText(t('support.session.key_label'));
		await fireEvent.input(keyField, { target: { value: '  secret-key  ' } });
		const enabledSignIn = screen.getByRole('button', { name: t('support.session.sign_in') });
		expect(enabledSignIn.getAttribute('aria-disabled')).not.toBe('true');
		await fireEvent.click(enabledSignIn);
		expect(onreauth).toHaveBeenCalledWith('secret-key');
	});

	it('renders nothing while closed', () => {
		render(SessionExpiryDialog, {
			props: { open: false, reason: 'idle', secondsLeft: 120, onstay: noop, onsignout: noop, onreauth: noop }
		});
		expect(screen.queryByRole('alertdialog')).toBeNull();
	});
});

describe('HelpPanel', () => {
	const diagnostics = 'callfs 1.0.0 · /files/team · req-1';

	it('lists glossary terms in a named dialog', () => {
		render(HelpPanel, { props: { open: true, ontopic: noop, diagnostics } });
		expect(screen.getByRole('dialog', { name: t('support.help.title') })).toBeTruthy();
		expect(screen.getByRole('button', { name: GLOSSARY.file.word })).toBeTruthy();
	});

	it('filters terms by the search text', async () => {
		render(HelpPanel, { props: { open: true, ontopic: noop, diagnostics } });
		await fireEvent.input(screen.getByRole('searchbox', { name: t('support.help.search_label') }), {
			target: { value: 'quorum' }
		});
		expect(screen.getByRole('button', { name: GLOSSARY.quorum.word })).toBeTruthy();
		expect(screen.queryByRole('button', { name: GLOSSARY.file.word })).toBeNull();
	});

	it('explains that nothing matches the search text', async () => {
		render(HelpPanel, { props: { open: true, ontopic: noop, diagnostics } });
		await fireEvent.input(screen.getByRole('searchbox', { name: t('support.help.search_label') }), {
			target: { value: 'zzzz-none' }
		});
		expect(screen.getByText(t('support.help.no_match', { query: 'zzzz-none' }))).toBeTruthy();
	});

	it('reports the chosen term through ontopic', async () => {
		const ontopic = vi.fn();
		render(HelpPanel, { props: { open: true, ontopic, diagnostics } });
		await fireEvent.click(screen.getByRole('button', { name: GLOSSARY.file.word }));
		expect(ontopic).toHaveBeenCalledWith('file');
	});

	it('shows the topic term for this page and marks it pressed in the list', () => {
		render(HelpPanel, { props: { open: true, topic: 'cluster-member', ontopic: noop, diagnostics } });
		const topic = screen.getByRole('region', { name: t('support.help.topic_label') });
		expect(topic.textContent).toContain(GLOSSARY.cluster_member.word);
		expect(screen.getByRole('button', { name: GLOSSARY.cluster_member.word }).getAttribute('aria-pressed')).toBe(
			'true'
		);
	});

	it('lists the screen shortcuts with their keys', () => {
		render(HelpPanel, {
			props: { open: true, ontopic: noop, diagnostics, shortcuts: [{ label: 'Open help', keys: ['?'] }] }
		});
		expect(screen.getByRole('region', { name: t('support.help.shortcuts_label') })).toBeTruthy();
		expect(screen.getByText('Open help')).toBeTruthy();
	});

	it('offers Report a problem as a copy action', () => {
		render(HelpPanel, { props: { open: true, ontopic: noop, diagnostics } });
		expect(screen.getByRole('button', { name: t('support.help.report') })).toBeTruthy();
	});
});
