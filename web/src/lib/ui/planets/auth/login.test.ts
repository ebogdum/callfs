import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Capabilities, Me } from '$lib/api/types';
import { t } from '$lib/copy/t';
import type { UiError } from '$lib/ui/types';
import LoginPlanet from './LoginPlanet.svelte';

const sessionApi = vi.hoisted(() => ({
	signIn: vi.fn(),
	signOut: vi.fn(),
	me: vi.fn(),
	capabilities: vi.fn()
}));

const navigation = vi.hoisted(() => ({ goto: vi.fn() }));

const appState = vi.hoisted(() => ({ page: { url: new URL('http://127.0.0.1:4173/ui/login') } }));

vi.mock('$lib/api/session', () => sessionApi);
vi.mock('$app/navigation', () => navigation);
vi.mock('$app/paths', () => ({ base: '/ui' }));
vi.mock('$app/state', () => appState);

const GRANT = {
	user_id: 'alice',
	is_admin: false,
	expires_at: '2030-01-01T00:00:00Z',
	csrf_token: 'csrf-token'
};

const ME: Me = {
	user_id: 'alice',
	is_admin: false,
	auth_method: 'session',
	session_expires_at: '2030-01-01T00:00:00Z',
	csrf_token: 'csrf-token',
	permissions_model: 'owner'
};

const CAPABILITIES = {} as Capabilities;

function failure(overrides: Partial<UiError>): { ok: false; error: UiError } {
	return {
		ok: false,
		error: { title: 'Request failed.', body: 'Try again later.', retryable: false, ...overrides }
	};
}

function keyField(): HTMLElement {
	return screen.getByLabelText(t('login.key.label'));
}

function submitButton(): HTMLElement {
	return screen.getByRole('button', { name: t('login.submit') });
}

async function typeKey(value: string): Promise<void> {
	await fireEvent.input(keyField(), { target: { value } });
}

beforeEach(() => {
	vi.clearAllMocks();
	appState.page.url = new URL('http://127.0.0.1:4173/ui/login');
	sessionApi.signIn.mockResolvedValue({ ok: true, value: GRANT });
	sessionApi.me.mockResolvedValue({ ok: true, value: ME });
	sessionApi.capabilities.mockResolvedValue({ ok: true, value: CAPABILITIES });
	navigation.goto.mockResolvedValue(undefined);
});

describe('LoginPlanet', () => {
	it('renders one level-1 heading and exactly one primary button', () => {
		const { container } = render(LoginPlanet);

		expect(screen.getByRole('heading', { level: 1, name: t('login.title') })).toBeInTheDocument();
		const primaries = container.querySelectorAll('button[data-variant="primary"]');
		expect(primaries).toHaveLength(1);
		expect(primaries[0]).toHaveTextContent(t('login.submit'));
	});

	it('labels the access key field and masks the typed key with current-password autocomplete', () => {
		render(LoginPlanet);

		expect(keyField()).toHaveAttribute('type', 'password');
		expect(keyField()).toHaveAttribute('autocomplete', 'current-password');
	});

	it('signs in with the typed key, loads the session and capabilities, then opens the default page', async () => {
		render(LoginPlanet);

		await typeKey('key-123');
		await fireEvent.click(submitButton());

		await waitFor(() => expect(navigation.goto).toHaveBeenCalledWith('/ui/'));
		expect(sessionApi.signIn).toHaveBeenCalledWith('key-123');
		expect(sessionApi.me).toHaveBeenCalledTimes(1);
		expect(sessionApi.capabilities).toHaveBeenCalledTimes(1);
	});

	it('submits when the form is submitted from the key field (Enter)', async () => {
		render(LoginPlanet);

		await typeKey('key-456');
		await fireEvent.submit(keyField());

		await waitFor(() => expect(sessionApi.signIn).toHaveBeenCalledWith('key-456'));
	});

	it('returns to the redirect parameter after sign-in', async () => {
		appState.page.url = new URL('http://127.0.0.1:4173/ui/login?redirect=%2Ffiles%2Fteam');
		render(LoginPlanet);

		await typeKey('key-123');
		await fireEvent.click(submitButton());

		await waitFor(() => expect(navigation.goto).toHaveBeenCalledWith('/ui/files/team'));
	});

	it('ignores a redirect parameter that points to another origin', async () => {
		appState.page.url = new URL('http://127.0.0.1:4173/ui/login?redirect=%2F%2Fevil.example');
		render(LoginPlanet);

		await typeKey('key-123');
		await fireEvent.click(submitButton());

		await waitFor(() => expect(navigation.goto).toHaveBeenCalledWith('/ui/'));
	});

	it('calls onsignedin with the signed-in user instead of navigating', async () => {
		const onsignedin = vi.fn();
		render(LoginPlanet, { props: { onsignedin } });

		await typeKey('key-123');
		await fireEvent.click(submitButton());

		await waitFor(() => expect(onsignedin).toHaveBeenCalledWith(ME));
		expect(navigation.goto).not.toHaveBeenCalled();
	});

	it('marks the sign-in button busy while the request is pending and clears it after the response', async () => {
		let resolveSignIn: (value: unknown) => void = () => undefined;
		sessionApi.signIn.mockReturnValue(
			new Promise((resolve) => {
				resolveSignIn = resolve;
			})
		);
		render(LoginPlanet);

		await typeKey('key-123');
		await fireEvent.click(submitButton());

		await waitFor(() => expect(submitButton()).toHaveAttribute('aria-busy', 'true'));
		resolveSignIn(failure({ status: 401, layer: 'auth' }));
		await waitFor(() => expect(submitButton()).not.toHaveAttribute('aria-busy'));
	});

	it('shows the rejected-key message under the field when the server answers 401', async () => {
		sessionApi.signIn.mockResolvedValue(failure({ status: 401, layer: 'auth', title: 'You are signed out.' }));
		render(LoginPlanet);

		await typeKey('wrong-key');
		await fireEvent.click(submitButton());

		expect(await screen.findByText(t('login.error.invalid_key'))).toBeInTheDocument();
		expect(navigation.goto).not.toHaveBeenCalled();
	});

	it('shows the rate limit message with Retry, and Retry submits the same key again', async () => {
		sessionApi.signIn.mockResolvedValue(
			failure({ status: 429, layer: 'limit', title: 'Too many requests.', retryable: true })
		);
		render(LoginPlanet);

		await typeKey('key-123');
		await fireEvent.click(submitButton());

		expect(await screen.findByText(t('login.error.rate_limited'))).toBeInTheDocument();
		await fireEvent.click(screen.getByRole('button', { name: t('common.retry') }));

		await waitFor(() => expect(sessionApi.signIn).toHaveBeenCalledTimes(2));
		expect(sessionApi.signIn).toHaveBeenLastCalledWith('key-123');
	});

	it('shows the unreachable message with Retry when the network fails', async () => {
		sessionApi.signIn.mockResolvedValue(
			failure({ layer: 'network', title: 'We cannot reach callfs.', retryable: true })
		);
		render(LoginPlanet);

		await typeKey('key-123');
		await fireEvent.click(submitButton());

		expect(await screen.findByText(t('login.error.unreachable'))).toBeInTheDocument();
		expect(screen.getByRole('button', { name: t('common.retry') })).toBeInTheDocument();
	});

	it('shows other failures with their own title and body and no Retry when not retryable', async () => {
		sessionApi.signIn.mockResolvedValue(
			failure({
				status: 500,
				layer: 'server',
				title: 'The server failed to sign in.',
				body: 'Nothing else changed. Try again, or copy the details for your admin.',
				retryable: false
			})
		);
		render(LoginPlanet);

		await typeKey('key-123');
		await fireEvent.click(submitButton());

		expect(
			await screen.findByText('The server failed to sign in.')
		).toBeInTheDocument();
		expect(screen.getByText('Nothing else changed. Try again, or copy the details for your admin.')).toBeInTheDocument();
		expect(screen.queryByRole('button', { name: t('common.retry') })).toBeNull();
	});

	it('opens the help panel from the "Where do I find it?" button', async () => {
		render(LoginPlanet);

		await fireEvent.click(screen.getByRole('button', { name: t('login.key.where') }));

		expect(await screen.findByRole('dialog')).toBeInTheDocument();
	});
});
