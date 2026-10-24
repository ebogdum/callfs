import { toUiError, type ApiFailure } from '$lib/api/errors';
import { upload, wsDownload } from '$lib/api/files';
import type { Result } from '$lib/api/types';
import { abortUpload, createUpload, getUpload, uploadResumable } from '$lib/api/uploads';
import type { UiError } from '$lib/ui/types';

export type TransferDirection = 'upload' | 'download';
export type TransferState =
	| 'queued'
	| 'uploading'
	| 'downloading'
	| 'paused'
	| 'stalled'
	| 'verifying'
	| 'done'
	| 'failed'
	| 'cancelled'
	| 'conflict';
export type ConflictPolicy = 'skip' | 'replace' | 'keep-both' | 'replace-if-changed';

export interface ExistingFile {
	size: number;
	etag: string;
}

export interface Transfer {
	id: string;
	direction: TransferDirection;
	name: string;
	destination: string;
	state: TransferState;
	done: number;
	total: number | null;
	partsDone: number | null;
	partsTotal: number | null;
	bytesPerSecond: number;
	eta: number | null;
	resumeId: string | null;
	resumedAt: number | null;
	existing: ExistingFile | null;
	conflictPolicy: ConflictPolicy | null;
	error: UiError | null;
}

export interface TransferAggregate {
	done: number;
	total: number | null;
	bytesPerSecond: number;
	eta: number | null;
}

export interface TransferSettings {
	parallel: number;
	partBytes: number;
}

export interface ResumableRecord {
	uploadId: string;
	directory: string;
	name: string;
	size: number;
}

export interface EnqueueResult {
	queued: string[];
	rejected: File[];
}

export type ConflictPlan =
	| { action: 'choose' }
	| { action: 'skip' }
	| { action: 'upload'; name: string; overwrite: boolean; ifMatch: string | null };

interface UploadContext {
	file: File;
	directory: string;
	listing: ReadonlyMap<string, ExistingFile>;
	overwrite: boolean;
	ifMatch: string | null;
}

interface Sample {
	at: number;
	done: number;
}

const MIB = 1024 * 1024;
const RESUMABLE_BYTES = 64 * MIB;
const MAX_UPLOAD_BYTES = 10 * 1024 * MIB;
const DEFAULT_PART_BYTES = 8 * MIB;
const MIN_PART_BYTES = MIB;
const MAX_PART_BYTES = 64 * MIB;
const DEFAULT_PARALLEL = 3;
const MIN_PARALLEL = 1;
const MAX_PARALLEL = 6;
const STALL_MS = 15_000;
const TICK_MS = 1_000;
const THROUGHPUT_ALPHA = 0.3;
const RECORDS_KEY = 'callfs.transfers.resumable';
const ACTIVE_STATES: readonly TransferState[] = ['uploading', 'downloading', 'stalled', 'verifying'];
const MOVING_STATES: readonly TransferState[] = ['uploading', 'downloading'];
const FINISHED_STATES: readonly TransferState[] = ['done', 'failed', 'cancelled'];
const VERIFY_BYTES = RESUMABLE_BYTES;
const NO_LISTING: ReadonlyMap<string, ExistingFile> = new Map();

export function createTransfer(
	fields: Pick<Transfer, 'id' | 'direction' | 'name' | 'destination' | 'total' | 'existing'>
): Transfer {
	return {
		...fields,
		state: 'queued',
		done: 0,
		partsDone: null,
		partsTotal: null,
		bytesPerSecond: 0,
		eta: null,
		resumeId: null,
		resumedAt: null,
		conflictPolicy: null,
		error: null
	};
}

export function joinPath(directory: string, name: string): string {
	return directory.endsWith('/') ? `${directory}${name}` : `${directory}/${name}`;
}

export function relativeDirectory(file: File): string {
	const cut = (file.webkitRelativePath ?? '').lastIndexOf('/');
	return 0 < cut ? file.webkitRelativePath.slice(0, cut) : '';
}

export function keepBothName(name: string, listing: ReadonlyMap<string, ExistingFile>): string {
	const dot = name.lastIndexOf('.');
	const stem = 0 < dot ? name.slice(0, dot) : name;
	const extension = 0 < dot ? name.slice(dot) : '';
	let counter = 2;
	while (listing.has(`${stem} (${counter})${extension}`)) {
		counter += 1;
	}
	return `${stem} (${counter})${extension}`;
}

export function planConflict(
	policy: ConflictPolicy | null,
	name: string,
	size: number,
	listing: ReadonlyMap<string, ExistingFile>
): ConflictPlan {
	const existing = listing.get(name) ?? null;
	if (null === existing) {
		return { action: 'upload', name, overwrite: false, ifMatch: null };
	}
	if (null === policy) {
		return { action: 'choose' };
	}
	if ('skip' === policy) {
		return { action: 'skip' };
	}
	if ('replace' === policy) {
		return { action: 'upload', name, overwrite: true, ifMatch: null };
	}
	if ('keep-both' === policy) {
		return { action: 'upload', name: keepBothName(name, listing), overwrite: false, ifMatch: null };
	}
	if (size === existing.size) {
		return { action: 'skip' };
	}
	return { action: 'upload', name, overwrite: true, ifMatch: '' === existing.etag ? null : existing.etag };
}

export function nextThroughput(previous: number | null, bytes: number, seconds: number): number {
	if (0 >= seconds) {
		return previous ?? 0;
	}
	const sample = Math.max(0, bytes) / seconds;
	return null === previous ? sample : THROUGHPUT_ALPHA * sample + (1 - THROUGHPUT_ALPHA) * previous;
}

export function etaSeconds(remaining: number | null, bytesPerSecond: number): number | null {
	if (null === remaining) {
		return null;
	}
	if (0 >= remaining) {
		return 0;
	}
	if (0 >= bytesPerSecond) {
		return null;
	}
	return remaining / bytesPerSecond;
}

export function summarize(items: readonly Transfer[]): TransferAggregate {
	let done = 0;
	let total = 0;
	let remaining = 0;
	let bytesPerSecond = 0;
	let totalKnown = true;
	for (const item of items) {
		if ('cancelled' === item.state) {
			continue;
		}
		done += item.done;
		if (null === item.total) {
			totalKnown = false;
		} else {
			total += item.total;
			if (false === FINISHED_STATES.includes(item.state)) {
				remaining += Math.max(0, item.total - item.done);
			}
		}
		if (ACTIVE_STATES.includes(item.state)) {
			bytesPerSecond += item.bytesPerSecond;
		}
	}
	return {
		done,
		total: totalKnown ? total : null,
		bytesPerSecond,
		eta: totalKnown ? etaSeconds(remaining, bytesPerSecond) : null
	};
}

function clampInteger(value: number, min: number, max: number): number {
	if (false === Number.isFinite(value)) {
		return min;
	}
	return Math.min(max, Math.max(min, Math.trunc(value)));
}

function partsFor(
	direction: TransferDirection,
	total: number | null,
	done: number,
	partBytes: number
): Pick<Transfer, 'partsDone' | 'partsTotal'> {
	if ('upload' !== direction || null === total || RESUMABLE_BYTES >= total) {
		return { partsDone: null, partsTotal: null };
	}
	return { partsDone: Math.floor(done / partBytes), partsTotal: Math.ceil(total / partBytes) };
}

function parseJson(raw: string): Result<unknown, string> {
	try {
		return { ok: true, value: JSON.parse(raw) };
	} catch (error) {
		return { ok: false, error: String(error) };
	}
}

function isRecord(value: unknown): value is ResumableRecord {
	if ('object' !== typeof value || null === value) {
		return false;
	}
	const candidate = value as Record<string, unknown>;
	return (
		'string' === typeof candidate.uploadId &&
		'string' === typeof candidate.directory &&
		'string' === typeof candidate.name &&
		'number' === typeof candidate.size
	);
}

export function parseRecords(raw: string | null): ResumableRecord[] {
	if (null === raw) {
		return [];
	}
	const parsed = parseJson(raw);
	if (!parsed.ok || !Array.isArray(parsed.value)) {
		return [];
	}
	return parsed.value.filter(isRecord);
}

function readRecords(): ResumableRecord[] {
	if ('undefined' === typeof sessionStorage) {
		return [];
	}
	return parseRecords(sessionStorage.getItem(RECORDS_KEY));
}

function writeRecords(records: readonly ResumableRecord[]): void {
	if ('undefined' === typeof sessionStorage) {
		return;
	}
	sessionStorage.setItem(RECORDS_KEY, JSON.stringify(records));
}

function remoteEtag(value: unknown): string | null {
	if ('object' !== typeof value || null === value) {
		return null;
	}
	const etag = (value as Record<string, unknown>).etag;
	if ('string' !== typeof etag || '' === etag) {
		return null;
	}
	return etag.replace(/^W\//, '').replaceAll('"', '').toLowerCase();
}

async function sha256Hex(file: Blob): Promise<Result<string, string>> {
	try {
		const digest = await globalThis.crypto.subtle.digest('SHA-256', await file.arrayBuffer());
		const hex = Array.from(new Uint8Array(digest), (byte) => byte.toString(16).padStart(2, '0')).join('');
		return { ok: true, value: hex };
	} catch (error) {
		return { ok: false, error: String(error) };
	}
}

function saveBlob(name: string, blob: Blob): void {
	const url = URL.createObjectURL(blob);
	const anchor = document.createElement('a');
	anchor.href = url;
	anchor.download = name;
	anchor.click();
	setTimeout(() => {
		URL.revokeObjectURL(url);
	}, TICK_MS);
}

export class TransferStore {
	items = $state<Transfer[]>([]);
	settings = $state<TransferSettings>({ parallel: DEFAULT_PARALLEL, partBytes: DEFAULT_PART_BYTES });
	resumable = $state<ResumableRecord[]>(readRecords());
	aggregate = $derived(summarize(this.items));

	private readonly controllers = new Map<string, AbortController>();
	private readonly contexts = new Map<string, UploadContext>();
	private readonly samples = new Map<string, Sample>();
	private ticker: ReturnType<typeof setInterval> | null = null;
	private sequence = 0;

	enqueueUploads(
		files: readonly File[],
		directory: string,
		listing: ReadonlyMap<string, ExistingFile>,
		policy: ConflictPolicy | null
	): EnqueueResult {
		const queued: string[] = [];
		const rejected: File[] = [];
		for (const file of files) {
			if (MAX_UPLOAD_BYTES < file.size) {
				rejected.push(file);
				continue;
			}
			const id = this.nextId();
			const nested = relativeDirectory(file);
			const target = '' === nested ? directory : joinPath(directory, nested);
			const targetListing = '' === nested ? listing : NO_LISTING;
			this.items = [
				...this.items,
				createTransfer({
					id,
					direction: 'upload',
					name: file.name,
					destination: joinPath(target, file.name),
					total: file.size,
					existing: targetListing.get(file.name) ?? null
				})
			];
			const context: UploadContext = { file, directory: target, listing: targetListing, overwrite: false, ifMatch: null };
			this.contexts.set(id, context);
			this.place(id, context, policy);
			queued.push(id);
		}
		this.pump();
		return { queued, rejected };
	}

	download(path: string, name: string, size: number | null): string {
		const id = this.nextId();
		this.items = [
			...this.items,
			createTransfer({ id, direction: 'download', name, destination: path, total: size, existing: null })
		];
		this.pump();
		return id;
	}

	resolveConflict(id: string, policy: ConflictPolicy): void {
		const item = this.find(id);
		const context = this.contexts.get(id);
		if (null === item || undefined === context || 'conflict' !== item.state) {
			return;
		}
		this.place(id, context, policy);
		this.pump();
	}

	pause(id: string): void {
		const item = this.find(id);
		if (null === item || 'upload' !== item.direction || false === ACTIVE_STATES.includes(item.state)) {
			return;
		}
		this.abort(id);
		this.patch(id, { state: 'paused', bytesPerSecond: 0, eta: null });
		this.pump();
	}

	resume(id: string): void {
		const item = this.find(id);
		if (null === item || 'paused' !== item.state) {
			return;
		}
		this.requeue(id, item);
	}

	retry(id: string): void {
		const item = this.find(id);
		if (null === item || ('failed' !== item.state && 'cancelled' !== item.state)) {
			return;
		}
		this.requeue(id, item);
	}

	retryFailed(): void {
		for (const item of this.items) {
			if ('failed' === item.state) {
				this.requeue(item.id, item);
			}
		}
	}

	async cancel(id: string): Promise<void> {
		const item = this.find(id);
		if (null === item || 'done' === item.state || 'cancelled' === item.state) {
			return;
		}
		this.abort(id);
		this.patch(id, { state: 'cancelled', bytesPerSecond: 0, eta: null, resumeId: null, error: null });
		this.forgetRecord(item.resumeId);
		this.pump();
		if (null === item.resumeId) {
			return;
		}
		const result = await abortUpload(item.resumeId);
		if (!result.ok) {
			this.patch(id, { error: toUiError(result.error, { action: 'files.upload', object: item.name }) });
		}
	}

	clearFinished(): void {
		for (const item of this.items) {
			if (FINISHED_STATES.includes(item.state)) {
				this.contexts.delete(item.id);
			}
		}
		this.items = this.items.filter((item) => false === FINISHED_STATES.includes(item.state));
	}

	setParallel(value: number): void {
		this.settings.parallel = clampInteger(value, MIN_PARALLEL, MAX_PARALLEL);
		this.pump();
	}

	setPartBytes(value: number): void {
		this.settings.partBytes = clampInteger(value, MIN_PART_BYTES, MAX_PART_BYTES);
	}

	async resumeFrom(uploadId: string, file: File): Promise<Result<string, 'not_found' | 'size_mismatch' | 'expired'>> {
		const record = this.resumable.find((entry) => uploadId === entry.uploadId);
		if (undefined === record) {
			return { ok: false, error: 'not_found' };
		}
		if (record.size !== file.size) {
			return { ok: false, error: 'size_mismatch' };
		}
		const session = await getUpload(uploadId);
		if (!session.ok || 'open' !== session.value.state) {
			this.forgetRecord(uploadId);
			return { ok: false, error: 'expired' };
		}
		const id = this.nextId();
		const offset = session.value.offset;
		this.items = [
			...this.items,
			{
				...createTransfer({
					id,
					direction: 'upload',
					name: record.name,
					destination: joinPath(record.directory, record.name),
					total: file.size,
					existing: null
				}),
				done: offset,
				resumeId: uploadId,
				resumedAt: 0 < offset ? offset : null
			}
		];
		this.contexts.set(id, { file, directory: record.directory, listing: NO_LISTING, overwrite: false, ifMatch: null });
		this.pump();
		return { ok: true, value: id };
	}

	async discardResumable(uploadId: string): Promise<Result<unknown, ApiFailure>> {
		this.forgetRecord(uploadId);
		return abortUpload(uploadId);
	}

	private find(id: string): Transfer | null {
		return this.items.find((item) => id === item.id) ?? null;
	}

	private nextId(): string {
		this.sequence += 1;
		return `transfer-${this.sequence}`;
	}

	private patch(id: string, changes: Partial<Transfer>): void {
		this.items = this.items.map((item) => (id === item.id ? { ...item, ...changes } : item));
	}

	private place(id: string, context: UploadContext, policy: ConflictPolicy | null): void {
		const plan = planConflict(policy, context.file.name, context.file.size, context.listing);
		if ('choose' === plan.action) {
			this.patch(id, { state: 'conflict', conflictPolicy: null, error: null });
			return;
		}
		if ('skip' === plan.action) {
			this.patch(id, { state: 'cancelled', conflictPolicy: policy, error: null });
			return;
		}
		context.overwrite = plan.overwrite;
		context.ifMatch = plan.ifMatch;
		this.patch(id, {
			name: plan.name,
			destination: joinPath(context.directory, plan.name),
			state: 'queued',
			conflictPolicy: policy,
			error: null
		});
	}

	private requeue(id: string, item: Transfer): void {
		this.patch(id, {
			state: 'queued',
			error: null,
			bytesPerSecond: 0,
			eta: null,
			done: null === item.resumeId ? 0 : item.done
		});
		this.pump();
	}

	private abort(id: string): void {
		this.controllers.get(id)?.abort();
		this.controllers.delete(id);
		this.samples.delete(id);
	}

	private pump(): void {
		const running = this.items.filter((item) => ACTIVE_STATES.includes(item.state)).length;
		const slots = Math.max(0, this.settings.parallel - running);
		for (const item of this.items.filter((entry) => 'queued' === entry.state).slice(0, slots)) {
			this.start(item.id);
		}
		this.syncTicker();
	}

	private start(id: string): void {
		const item = this.find(id);
		if (null === item) {
			return;
		}
		const controller = new AbortController();
		this.controllers.set(id, controller);
		this.samples.set(id, { at: Date.now(), done: item.done });
		this.patch(id, { state: 'upload' === item.direction ? 'uploading' : 'downloading', error: null });
		void this.run(id, controller);
	}

	private async run(id: string, controller: AbortController): Promise<void> {
		const item = this.find(id);
		if (null === item) {
			return;
		}
		if ('download' === item.direction) {
			const result = await wsDownload(item.destination, {
				signal: controller.signal,
				onProgress: (received, total) => {
					this.progress(id, controller, received, total);
				}
			});
			if (result.ok && false === controller.signal.aborted) {
				saveBlob(item.name, result.value);
			}
			this.settle(id, controller, result);
			return;
		}
		const context = this.contexts.get(id);
		if (undefined === context) {
			this.settle(id, controller, { ok: false, error: { kind: 'malformed' } });
			return;
		}
		if (RESUMABLE_BYTES < context.file.size) {
			await this.runResumable(id, item, context, controller);
			return;
		}
		const result = await upload(item.destination, context.file, {
			overwrite: context.overwrite,
			ifMatch: context.ifMatch,
			signal: controller.signal,
			onProgress: (loaded, total) => {
				this.progress(id, controller, loaded, total);
			}
		});
		this.settle(id, controller, result);
	}

	private async runResumable(
		id: string,
		item: Transfer,
		context: UploadContext,
		controller: AbortController
	): Promise<void> {
		let uploadId = item.resumeId;
		if (null === uploadId) {
			const created = await createUpload(item.destination, context.file.size, context.overwrite);
			if (!created.ok) {
				this.settle(id, controller, created);
				return;
			}
			uploadId = created.value.upload_id;
			this.patch(id, { resumeId: uploadId });
			this.remember({ uploadId, directory: context.directory, name: item.name, size: context.file.size });
		}
		const result = await uploadResumable(item.destination, context.file, {
			partBytes: this.settings.partBytes,
			overwrite: context.overwrite,
			resumeId: uploadId,
			signal: controller.signal,
			onProgress: (sent, total) => {
				this.progress(id, controller, sent, total);
			}
		});
		this.settle(id, controller, result);
	}

	private progress(id: string, controller: AbortController, done: number, total: number | null): void {
		const item = this.find(id);
		if (null === item || controller.signal.aborted) {
			return;
		}
		const now = Date.now();
		const previous = this.samples.get(id) ?? { at: now, done };
		const previousRate = 0 < item.bytesPerSecond ? item.bytesPerSecond : null;
		const bytesPerSecond = nextThroughput(previousRate, done - previous.done, (now - previous.at) / 1000);
		const knownTotal = total ?? item.total;
		this.samples.set(id, { at: now, done });
		this.patch(id, {
			state: 'upload' === item.direction ? 'uploading' : 'downloading',
			done,
			total: knownTotal,
			bytesPerSecond,
			eta: etaSeconds(null === knownTotal ? null : Math.max(0, knownTotal - done), bytesPerSecond),
			...partsFor(item.direction, knownTotal, done, this.settings.partBytes)
		});
	}

	private settle(id: string, controller: AbortController, result: Result<unknown, ApiFailure>): void {
		const item = this.find(id);
		if (controller.signal.aborted || null === item) {
			return;
		}
		this.controllers.delete(id);
		if (!result.ok) {
			this.fail(id, item, result.error);
			this.pump();
			return;
		}
		const context = this.contexts.get(id);
		const etag = remoteEtag(result.value);
		if ('upload' === item.direction && undefined !== context && null !== etag && VERIFY_BYTES >= context.file.size) {
			void this.verify(id, item, context.file, etag);
			return;
		}
		this.complete(id, item);
		this.pump();
	}

	private async verify(id: string, item: Transfer, file: Blob, etag: string): Promise<void> {
		this.samples.delete(id);
		this.patch(id, { state: 'verifying', done: item.total ?? item.done, bytesPerSecond: 0, eta: null });
		const local = await sha256Hex(file);
		const current = this.find(id);
		if (null === current || 'verifying' !== current.state) {
			return;
		}
		if (local.ok && local.value !== etag) {
			this.fail(id, current, { kind: 'malformed' });
		} else {
			this.complete(id, current);
		}
		this.pump();
	}

	private complete(id: string, item: Transfer): void {
		this.samples.delete(id);
		this.contexts.delete(id);
		this.forgetRecord(item.resumeId);
		this.patch(id, { state: 'done', done: item.total ?? item.done, bytesPerSecond: 0, eta: null, error: null });
	}

	private fail(id: string, item: Transfer, failure: ApiFailure): void {
		this.samples.delete(id);
		if ('upload' === item.direction && 'http' === failure.kind && 409 === failure.status && 'RESOURCE_LOCKED' !== failure.code) {
			const context = this.contexts.get(id);
			const known = context?.listing.get(item.name) ?? null;
			if (undefined !== context && null === known) {
				context.listing = new Map([...context.listing, [item.name, { size: -1, etag: '' }]]);
			}
			this.patch(id, {
				state: 'conflict',
				conflictPolicy: null,
				existing: known,
				bytesPerSecond: 0,
				eta: null,
				error: null
			});
			return;
		}
		this.patch(id, {
			state: 'failed',
			bytesPerSecond: 0,
			eta: null,
			error: toUiError(failure, {
				action: 'upload' === item.direction ? 'files.upload' : 'files.download',
				object: item.name
			})
		});
	}

	private remember(record: ResumableRecord): void {
		this.resumable = [...this.resumable.filter((entry) => record.uploadId !== entry.uploadId), record];
		writeRecords(this.resumable);
	}

	private forgetRecord(uploadId: string | null): void {
		if (null === uploadId) {
			return;
		}
		this.resumable = this.resumable.filter((entry) => uploadId !== entry.uploadId);
		writeRecords(this.resumable);
	}

	private syncTicker(): void {
		const hasActive = this.items.some((item) => ACTIVE_STATES.includes(item.state));
		if (hasActive && null === this.ticker) {
			this.ticker = setInterval(() => {
				this.tick();
			}, TICK_MS);
		}
		if (false === hasActive && null !== this.ticker) {
			clearInterval(this.ticker);
			this.ticker = null;
		}
	}

	private tick(): void {
		const now = Date.now();
		for (const item of this.items) {
			if (false === MOVING_STATES.includes(item.state)) {
				continue;
			}
			const sample = this.samples.get(item.id);
			if (undefined === sample || STALL_MS >= now - sample.at) {
				continue;
			}
			this.patch(item.id, { state: 'stalled', bytesPerSecond: 0, eta: null });
		}
	}
}

export const transfers = new TransferStore();
