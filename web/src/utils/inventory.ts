import type { InventoryItem } from "../types";

/** A domain expiring within this many days is flagged. */
export const EXPIRY_WARN_DAYS = 90;

const DAY_MS = 24 * 60 * 60 * 1000;
const ISO_DATE = /^\d{4}-\d{2}-\d{2}/;

/**
 * When a domain item expires: from its registry lookup, else from the
 * manual `expiry` detail when that is a YYYY-MM-DD date (for registries
 * that publish no expiry, such as .eu and .ch). Null when unknown.
 */
export function itemExpiry(item: InventoryItem): string | null {
	if (item.type !== "domain") return null;
	const looked = item.domain_lookup?.expires_at;
	if (looked) return looked;
	const manual = item.details?.expiry;
	if (typeof manual === "string" && ISO_DATE.test(manual.trim())) {
		return manual.trim();
	}
	return null;
}

/** Whole days until a date; negative once it has passed. */
export function daysUntil(date: string): number {
	return Math.floor((new Date(date).getTime() - Date.now()) / DAY_MS);
}

/**
 * The badge for an expiry date: "expired", or "in N days" within
 * EXPIRY_WARN_DAYS; null when it is further away.
 */
export function expiryBadge(
	date: string | null | undefined,
): { cls: string; text: string } | null {
	if (!date) return null;
	const days = daysUntil(date);
	if (days < 0) return { cls: "error", text: "expired" };
	if (days <= EXPIRY_WARN_DAYS) {
		return {
			cls: "warning",
			text: days === 1 ? "in 1 day" : `in ${days} days`,
		};
	}
	return null;
}

/** The expiry filter of the inventory list. */
export type ExpiryFilter = "" | "soon" | "expired";

/** Whether an item passes the expiry filter. */
export function matchesExpiryFilter(
	item: InventoryItem,
	filter: ExpiryFilter,
): boolean {
	if (!filter) return true;
	const expiry = itemExpiry(item);
	if (!expiry) return false;
	const days = daysUntil(expiry);
	return filter === "expired" ? days < 0 : days <= EXPIRY_WARN_DAYS;
}

const text = (v: unknown): string =>
	typeof v === "string" ? v.trim() : typeof v === "number" ? String(v) : "";

/**
 * A server item's one-line summary for the list: IP address, size and
 * place, from whichever provider (or the manual details) has them.
 */
export function serverSummary(item: InventoryItem): string {
	const hz = item.provider_data?.hetzner ?? {};
	const sp = item.provider_data?.spinupwp ?? {};
	// The template's list field, or a hand-made single "ip_address".
	const manualIPs = item.details?.ip_addresses ?? item.details?.ip_address;
	const ip =
		text(hz.ipv4) ||
		text(sp.ip_address) ||
		(Array.isArray(manualIPs) ? text(manualIPs[0]) : text(manualIPs));
	const size = text(hz.server_type) || text(sp.size);
	const place = text(hz.city) || text(sp.region);
	return [ip, size, place].filter(Boolean).join(" · ");
}
