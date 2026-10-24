const WORD_PATTERN = /[A-Za-z0-9]+(?:['’-][A-Za-z0-9]+)*/g;
const SENTENCE_END_PATTERN = /[.!?]+/;
const VOWEL_GROUP_PATTERN = /[aeiouy]+/g;
const SILENT_E_PATTERN = /[^aeiouyl]e$/;

export function words(text: string): string[] {
	return text.match(WORD_PATTERN) ?? [];
}

export function sentences(text: string): string[] {
	return text
		.split(SENTENCE_END_PATTERN)
		.map((part) => part.trim())
		.filter((part) => 0 < part.length);
}

export function syllables(word: string): number {
	const letters = word.toLowerCase().replace(/[^a-z]/g, '');
	if (3 >= letters.length) {
		return 1;
	}
	const groups = letters.match(VOWEL_GROUP_PATTERN)?.length ?? 0;
	const silentEnding = SILENT_E_PATTERN.test(letters) && 1 < groups;
	return Math.max(1, silentEnding ? groups - 1 : groups);
}

export function wordsPerSentence(text: string): number {
	const sentenceCount = sentences(text).length;
	if (0 === sentenceCount) {
		return 0;
	}
	return words(text).length / sentenceCount;
}

export function fleschKincaid(text: string): number {
	const wordList = words(text);
	const sentenceCount = sentences(text).length;
	if (0 === wordList.length || 0 === sentenceCount) {
		return 0;
	}
	const syllableCount = wordList.reduce((total, word) => total + syllables(word), 0);
	return 0.39 * (wordList.length / sentenceCount) + 11.8 * (syllableCount / wordList.length) - 15.59;
}
