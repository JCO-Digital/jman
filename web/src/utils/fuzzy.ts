/**
 * How well a text matches a search, lower is better, or null when it
 * doesn't: every word of the query must occur in the text. A text that
 * starts with the query ranks first, then one where a word does, then any.
 */
export function matchScore(text: string, query: string): number | null {
	const t = text.toLowerCase();
	const q = query.trim().toLowerCase();
	if (!q) return 0;
	const words = q.split(/\s+/);
	if (!words.every((w) => t.includes(w))) return null;
	if (t === q) return 0;
	if (t.startsWith(q)) return 1;
	// A word of the text (after a space, dot, dash, slash) starts with it.
	if (new RegExp(`(^|[\\s.\\-/_])${escapeRegExp(words[0]!)}`).test(t)) {
		return 2;
	}
	return 3;
}

function escapeRegExp(s: string): string {
	return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
