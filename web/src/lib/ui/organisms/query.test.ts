import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import type { Facet } from '../types';
import FacetSidebar from './FacetSidebar.svelte';
import QueryBar from './QueryBar.svelte';
import {
	chipFromToken,
	chipText,
	completionAt,
	format,
	parse,
	removeRange,
	type QueryGrammar
} from './query-grammar';

const NOW = Date.parse('2026-10-23T00:00:00.000Z');
const MB = 1024 ** 2;
const GB = 1024 ** 3;

const grammar: QueryGrammar = {
	fields: [
		{ key: 'size', label: 'Size', hint: 'A number and a unit' },
		{ key: 'type', label: 'Type', hint: 'File or folder' },
		{ key: 'name', label: 'Name', hint: 'A name pattern' }
	]
};

const storageFacet: Facet = {
	key: 'storage',
	label: 'Storage',
	values: [
		{ value: 's3', label: 'S3 bucket', count: 12 },
		{ value: 'localfs', label: 'Local disk' }
	]
};

describe('parse', () => {
	it('reads name and size tokens with their positions', () => {
		const result = parse('name:*.log size:>100MB', NOW);
		expect(result.errors).toEqual([]);
		expect(result.tokens.map((token) => [token.field, token.start, token.end])).toEqual([
			['name', 0, 10],
			['size', 11, 22]
		]);
		expect(result.query.name).toBe('*.log');
		expect(result.query.minSize).toBe(100 * MB + 1);
	});

	it('reads a size range with a unit on each side', () => {
		const { query } = parse('size:1MB..1GB', NOW);
		expect(query.minSize).toBe(MB);
		expect(query.maxSize).toBe(GB);
	});

	it('accepts units in any letter case', () => {
		expect(parse('size:1mb', NOW).query).toEqual(parse('size:1MB', NOW).query);
		expect(parse('size:1Mb', NOW).query.maxSize).toBe(MB);
	});

	it('maps size comparators to bounds', () => {
		expect(parse('size:>=2KB', NOW).query.minSize).toBe(2048);
		expect(parse('size:<=2KB', NOW).query.maxSize).toBe(2048);
		expect(parse('size:>2KB', NOW).query.minSize).toBe(2049);
		expect(parse('size:<2KB', NOW).query.maxSize).toBe(2047);
		const exact = parse('size:2KB', NOW).query;
		expect([exact.minSize, exact.maxSize]).toEqual([2048, 2048]);
	});

	it('reports a bad size unit and a reversed range', () => {
		expect(parse('size:12XB', NOW).errors.map((error) => error.reason)).toEqual(['bad_size']);
		expect(parse('size:5MB..1MB', NOW).errors.map((error) => error.reason)).toEqual(['bad_range']);
	});

	it('reads a relative span as a recent window and a date as a start day', () => {
		expect(parse('modified:<7d', NOW).query.modifiedAfter).toBe('2026-10-16T00:00:00.000Z');
		expect(parse('modified:>2026-01-01', NOW).query.modifiedAfter).toBe('2026-01-01T00:00:00.000Z');
		expect(parse('modified:<2026-01-01', NOW).query.modifiedBefore).toBe('2026-01-01T00:00:00.000Z');
	});

	it('rejects a date that does not exist', () => {
		expect(parse('modified:>2026-02-30', NOW).errors.map((error) => error.reason)).toEqual(['bad_date']);
	});

	it('maps folder to directory and rejects unknown types', () => {
		expect(parse('type:folder', NOW).query.type).toBe('directory');
		expect(parse('type:FILE', NOW).query.type).toBe('file');
		expect(parse('type:video', NOW).errors.map((error) => error.reason)).toEqual(['bad_value']);
	});

	it('reads yes and no words for split and locked', () => {
		const { query } = parse('split:YES locked:no', NOW);
		expect(query.split).toBe(true);
		expect(query.locked).toBe(false);
	});

	it('reads status comparators and rejects codes outside 100 to 599', () => {
		expect(parse('status:>=400', NOW).query.status).toEqual({ comparator: '>=', code: 400 });
		expect(parse('status:99', NOW).errors.map((error) => error.reason)).toEqual(['bad_status']);
	});

	it('accepts warning as warn for level', () => {
		expect(parse('level:Warning', NOW).query.level).toBe('warn');
	});

	it('reports unknown fields and empty values', () => {
		const unknown = parse('colour:red', NOW);
		expect(unknown.tokens).toEqual([]);
		expect(unknown.errors.map((error) => error.reason)).toEqual(['unknown_field']);
		expect(parse('owner:', NOW).errors.map((error) => error.reason)).toEqual(['empty_value']);
	});

	it('reads quoted values and reports a missing closing quote', () => {
		const quoted = parse('name:"my report.pdf" quarterly', NOW);
		expect(quoted.query.name).toBe('my report.pdf');
		expect(quoted.query.text).toBe('quarterly');
		expect(parse('name:"abc', NOW).errors.map((error) => error.reason)).toEqual(['unterminated_quote']);
	});

	it('moves negated tokens to exclude and keeps them as tokens', () => {
		const { query, tokens } = parse('!type:file', NOW);
		expect(query.type).toBeUndefined();
		expect(query.exclude.map((token) => [token.field, token.value])).toEqual([['type', 'file']]);
		expect(tokens[0]?.negated).toBe(true);
	});

	it('keeps free words as text', () => {
		expect(parse('quarterly report', NOW).query.text).toBe('quarterly report');
	});
});

describe('format', () => {
	it('writes tokens in grammar order and round trips through parse', () => {
		const text = format(parse('name:*.log type:file size:1MB..1GB', NOW).query);
		expect(text).toBe('name:*.log type:file size:1MB..1GB');
		expect(parse(text, NOW).query).toEqual(parse('name:*.log type:file size:1MB..1GB', NOW).query);
	});

	it('quotes text that has spaces and writes excluded tokens with a bang', () => {
		expect(format({ text: 'a b', exclude: [] })).toBe('"a b"');
		expect(format(parse('!storage:s3', NOW).query)).toBe('!storage:s3');
	});

	it('writes a status comparator and an exact status', () => {
		expect(format(parse('status:>=400', NOW).query)).toBe('status:>=400');
		expect(format(parse('status:404', NOW).query)).toBe('status:404');
	});
});

describe('completion and chips', () => {
	it('suggests a field name for a partial key', () => {
		const completion = completionAt('siz', 3, grammar, {});
		expect(completion?.options.map((option) => option.insert)).toEqual(['size:']);
		expect([completion?.start, completion?.end]).toEqual([0, 3]);
	});

	it('suggests values after a colon', () => {
		const completion = completionAt('type:', 5, grammar, { type: [{ value: 'file', label: 'File' }] });
		expect(completion?.options.map((option) => option.insert)).toEqual(['type:file']);
	});

	it('removes a token range and tidies the spacing', () => {
		expect(removeRange('a size:1MB b', 2, 11)).toBe('a b');
	});

	it('turns a token into a chip and back into the same text', () => {
		const [token] = parse('size:>1MB', NOW).tokens;
		if (undefined === token) {
			throw new Error('token missing');
		}
		const chip = chipFromToken(token);
		expect(chip).toEqual({ facet: 'size', op: ':', value: '>1MB', label: 'size:>1MB' });
		expect(chipText(chip)).toBe('size:>1MB');
	});
});

describe('QueryBar', () => {
	it('names the combobox with its visible label', () => {
		render(QueryBar, { props: { value: '', grammar, label: 'Search files' } });
		expect(screen.getByRole('combobox', { name: 'Search files' })).toBeTruthy();
	});

	it('submits the parsed query on Enter', async () => {
		const onsubmit = vi.fn();
		render(QueryBar, { props: { value: 'type:file', grammar, label: 'Search files', onsubmit } });
		await fireEvent.keyDown(screen.getByRole('combobox', { name: 'Search files' }), { key: 'Enter' });
		expect(onsubmit).toHaveBeenCalledTimes(1);
		expect(onsubmit.mock.calls[0]?.[0]?.query.type).toBe('file');
	});

	it('blocks submit and explains the invalid token', async () => {
		const onsubmit = vi.fn();
		render(QueryBar, { props: { value: 'colour:red', grammar, label: 'Search files', onsubmit } });
		await fireEvent.click(screen.getByRole('button', { name: 'Search' }));
		expect(onsubmit).not.toHaveBeenCalled();
		expect(screen.getByText('We do not know this field. Check the spelling.')).toBeTruthy();
	});

	it('offers a field name while typing and applies it with Enter', async () => {
		render(QueryBar, { props: { value: '', grammar, label: 'Search files' } });
		const box = screen.getByRole('combobox', { name: 'Search files' }) as HTMLInputElement;
		box.value = 'si';
		await fireEvent.input(box);
		expect(screen.getByRole('listbox')).toBeTruthy();
		await fireEvent.keyDown(box, { key: 'Enter' });
		expect(box.value).toBe('size:');
	});

	it('removes a chip from the text', async () => {
		render(QueryBar, { props: { value: 'type:file', grammar, label: 'Search files' } });
		await fireEvent.click(screen.getByRole('button', { name: 'Remove type:file' }));
		expect((screen.getByRole('combobox', { name: 'Search files' }) as HTMLInputElement).value).toBe('');
	});
});

describe('FacetSidebar', () => {
	it('checks an included value', async () => {
		render(FacetSidebar, { props: { facets: [storageFacet], label: 'Filters' } });
		const box = screen.getByRole('checkbox', { name: /S3 bucket/ }) as HTMLInputElement;
		await fireEvent.click(box);
		expect(box.checked).toBe(true);
	});

	it('excludes a value with Alt and shows the excluded word', async () => {
		render(FacetSidebar, { props: { facets: [storageFacet], label: 'Filters' } });
		const box = screen.getByRole('checkbox', { name: /Local disk/ }) as HTMLInputElement;
		await fireEvent.click(box, { altKey: true });
		expect(box.checked).toBe(false);
		expect(screen.getByText('Excluded')).toBeTruthy();
	});

	it('says there are no filters when no facets come back', () => {
		render(FacetSidebar, { props: { facets: [], label: 'Filters' } });
		expect(screen.getByText('No filters to show.')).toBeTruthy();
	});
});
