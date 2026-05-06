export interface PasswordStrengthResult {
	valid: boolean;
	score: number;
	poolSize: number;
	hasLowercase: boolean;
	hasUppercase: boolean;
	hasNumbers: boolean;
	hasSpecial: boolean;
}

const THRESHOLD = 200_000_000_000_000;
const LOG_THRESHOLD = Math.log(THRESHOLD);

function getPoolSize(
	hasLowercase: boolean,
	hasUppercase: boolean,
	hasNumbers: boolean,
	hasSpecial: boolean,
): number {
	let pool = 0;
	if (hasLowercase) pool += 26;
	if (hasUppercase) pool += 26;
	if (hasNumbers) pool += 10;
	if (hasSpecial) pool += 16;
	return pool;
}

export function validatePasswordStrength(
	password: string,
): PasswordStrengthResult {
	const hasLowercase = /[a-z]/.test(password);
	const hasUppercase = /[A-Z]/.test(password);
	const hasNumbers = /[0-9]/.test(password);
	const hasSpecial = /[^a-zA-Z0-9]/.test(password);

	const poolSize = getPoolSize(
		hasLowercase,
		hasUppercase,
		hasNumbers,
		hasSpecial,
	);

	if (password.length === 0 || poolSize === 0) {
		return {
			valid: false,
			score: 0,
			poolSize: 0,
			hasLowercase,
			hasUppercase,
			hasNumbers,
			hasSpecial,
		};
	}

	const logEntropy = password.length * Math.log(poolSize);
	const valid = logEntropy >= LOG_THRESHOLD;
	// Scale so the minimum requirement sits at ~33% of the meter,
	// allowing the bar to keep filling to show strength beyond the minimum.
	const score = Math.min(
		100,
		Math.round((logEntropy / (LOG_THRESHOLD * 3)) * 100),
	);

	return {
		valid,
		score,
		poolSize,
		hasLowercase,
		hasUppercase,
		hasNumbers,
		hasSpecial,
	};
}
