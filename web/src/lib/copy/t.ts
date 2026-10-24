export type CopyKey = string;

export type CopyParams = Record<string, string | number>;

type CopyDictionary = Record<string, string>;

const areaModules: Record<string, unknown> = import.meta.glob('./en/*.ts', { eager: true });

const pluralRules = new Intl.PluralRules('en');
const PLURAL_PATTERN = /\{(\w+), plural, one \{([^{}]*)\} other \{([^{}]*)\}\}/g;
const PLACEHOLDER_PATTERN = /\{(\w+)\}/g;

function isDictionaryModule(module: unknown): module is { default: CopyDictionary } {
	if ('object' !== typeof module || null === module || !('default' in module)) {
		return false;
	}
	const dictionary = module.default;
	if ('object' !== typeof dictionary || null === dictionary) {
		return false;
	}
	return Object.values(dictionary).every((value) => 'string' === typeof value);
}

const dictionary: CopyDictionary = {};
for (const module of Object.values(areaModules)) {
	if (isDictionaryModule(module)) {
		Object.assign(dictionary, module.default);
	}
}

function applyPlurals(message: string, params: CopyParams): string {
	return message.replace(PLURAL_PATTERN, (match: string, name: string, one: string, other: string) => {
		const count = params[name];
		if ('number' !== typeof count) {
			return match;
		}
		const branch = 'one' === pluralRules.select(count) ? one : other;
		return branch.replaceAll('#', String(count));
	});
}

function applyPlaceholders(message: string, params: CopyParams): string {
	return message.replace(PLACEHOLDER_PATTERN, (match: string, name: string) =>
		Object.hasOwn(params, name) ? String(params[name]) : match
	);
}

export function t(key: CopyKey, params: CopyParams = {}): string {
	if (!Object.hasOwn(dictionary, key)) {
		return key;
	}
	return applyPlaceholders(applyPlurals(dictionary[key], params), params);
}
