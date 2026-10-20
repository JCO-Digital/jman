const CHARS = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";

/**
 * A random password of letters and digits from the browser's CSPRNG; 24
 * characters give about 143 bits. Bytes that would bias the pick are
 * drawn again.
 */
export function generatePassword(length = 24): string {
	const limit = 256 - (256 % CHARS.length);
	let out = "";
	const buf = new Uint8Array(length * 2);
	while (out.length < length) {
		crypto.getRandomValues(buf);
		for (const b of buf) {
			if (b < limit && out.length < length)
				out += CHARS[b % CHARS.length];
		}
	}
	return out;
}
