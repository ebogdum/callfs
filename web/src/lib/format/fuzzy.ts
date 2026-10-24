export interface FuzzyMatch {
	score: number;
	indices: number[];
}

const WORD_BOUNDARY = /[\s/>\-_.@#?:]/;

const isWordStart = (text: string, index: number): boolean =>
	0 === index || WORD_BOUNDARY.test(text.charAt(index - 1));

export function fuzzyMatch(query: string, candidate: string): FuzzyMatch | null {
	const needle = query.trim().toLowerCase();
	if ('' === needle) {
		return { score: 0, indices: [] };
	}
	const haystack = candidate.toLowerCase();
	const indices: number[] = [];
	let score = 0;
	let from = 0;
	let previous = -2;
	for (const char of needle) {
		const found = haystack.indexOf(char, from);
		if (-1 === found) {
			return null;
		}
		indices.push(found);
		score += 1;
		if (found === previous + 1) {
			score += 4;
		}
		if (isWordStart(candidate, found)) {
			score += 3;
		}
		score -= (found - from) * 0.1;
		previous = found;
		from = found + 1;
	}
	if (0 === indices[0]) {
		score += 2;
	}
	score -= haystack.length * 0.01;
	return { score, indices };
}
