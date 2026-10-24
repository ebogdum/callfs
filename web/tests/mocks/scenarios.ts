import type { MockResponse, MockTable } from './server';
import { CLUSTER, fixtures, hugeFolderBody, ok } from './fixtures';

const BASE = '2026-10-23T12:00:00Z';
const KEPT_WHEN_ERRORING = ['GET /v1/me', 'GET /v1/capabilities'];
const SERVER_ERROR: MockResponse = { status: 500, body: { code: 'INTERNAL_ERROR', message: 'Something failed on the server.' } };
const NO_LEADER: MockResponse = { status: 503, body: { code: 'NO_LEADER', message: 'The cluster has no leader.' } };

const errorsEverywhere: MockTable = Object.fromEntries(
	Object.keys(fixtures)
		.filter((key) => key.startsWith('GET ') && !KEPT_WHEN_ERRORING.includes(key))
		.map((key) => [key, SERVER_ERROR])
);

export const scenarios: Record<string, MockTable> = {
	healthy: {},
	fresh_install: {
		'GET /v1/directories/*': ok({ path: '/', type: 'directory', count: 0, total: 0, items: [], next_cursor: null, partial: false }),
		'GET /v1/usage': ok({
			path: '/',
			files: 0,
			directories: 0,
			bytes: 0,
			by_storage: {},
			by_instance: {},
			by_owner: {},
			erasure_files: 0,
			erasure_raw_bytes: 0,
			children: [],
			size_histogram: [],
			age_histogram: [],
			computed_at: BASE,
			scan_truncated: false
		}),
		'GET /v1/links': ok({ items: [], next_cursor: null, counts: { active: 0, used: 0, expired: 0, revoked: 0 } }),
		'GET /v1/trash': ok({ items: [], next_cursor: null, total_bytes: 0 }),
		'GET /v1/erasure/files': ok({ items: [], next_cursor: null }),
		'GET /v1/admin/findings': ok({ items: [], counts: { critical: 0, warning: 0, info: 0 } })
	},
	degraded_no_leader: {
		'GET /v1/admin/cluster': ok({
			...CLUSTER,
			leader_id: null,
			leader_endpoint: null,
			has_quorum: false,
			can_lose: 0,
			members: CLUSTER.members.map((member) => ({ ...member, state: 'follower' }))
		}),
		'GET /v1/admin/health': ok({
			status: 'down',
			checked_at: BASE,
			components: [{ name: 'nats', status: 'down', latency_ms: null, detail: 'Not connected.', since: BASE }]
		}),
		'PUT /v1/files/*': NO_LEADER,
		'PATCH /v1/files/*': NO_LEADER,
		'DELETE /v1/files/*': NO_LEADER
	},
	member_user: {},
	errors_everywhere: errorsEverywhere,
	huge_folder: {
		'GET /v1/directories/*': ok(hugeFolderBody)
	}
};
