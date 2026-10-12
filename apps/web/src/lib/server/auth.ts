// Pemanggilan endpoint auth di Go API. Semua keputusan keamanan ada di Go (PRD §0 #13);
// di sini hanya meneruskan kredensial dan cookie sesi.
import type { Cookies } from '@sveltejs/kit';

const BASE = process.env.ERAPAY_API_URL ?? 'http://localhost:8080';
export const SESSION_COOKIE = 'erapay_session';

export type Me = { id: string; email: string; merchantId: string; permissions: string[] };

export type LoginResult =
	| { ok: true; token: string; maxAge: number }
	| { ok: false; code: 'invalid' | 'rate_limited' | 'merchant_required' | 'invalid_input' | 'unavailable' };

export async function login(
	f: typeof fetch,
	clientIp: string,
	email: string,
	password: string,
	merchantId?: string
): Promise<LoginResult> {
	let res: Response;
	try {
		res = await f(`${BASE}/v1/auth/login`, {
			method: 'POST',
			headers: { 'content-type': 'application/json', 'x-forwarded-for': clientIp },
			body: JSON.stringify({ email, password, merchant_id: merchantId || undefined })
		});
	} catch {
		return { ok: false, code: 'unavailable' };
	}
	if (res.status === 429) return { ok: false, code: 'rate_limited' };
	if (res.status === 401) return { ok: false, code: 'invalid' };
	if (res.status === 400) {
		const body = await res.json().catch(() => null);
		return { ok: false, code: /merchant_id/.test(body?.error?.message ?? '') ? 'merchant_required' : 'invalid_input' };
	}
	if (!res.ok) return { ok: false, code: 'unavailable' };
	for (const line of res.headers.getSetCookie()) {
		const m = line.match(new RegExp(`^${SESSION_COOKIE}=([^;]+)`));
		if (m) {
			const age = line.match(/Max-Age=(\d+)/i);
			return { ok: true, token: m[1], maxAge: age ? Number(age[1]) : 3600 };
		}
	}
	return { ok: false, code: 'unavailable' };
}

export type RegisterInput = {
	account_type: 'individual' | 'business';
	name: string;
	business_name: string;
	email: string;
	phone: string;
	password: string;
	accept_terms: boolean;
};
export type RegisterResult =
	| { ok: true }
	| { ok: false; code: 'email_taken' | 'rate_limited' | 'invalid' | 'unavailable'; message?: string };

/** Daftar merchant mandiri. Tidak membuat sesi: merchant baru berstatus "submitted" sampai disetujui admin. */
export async function register(f: typeof fetch, clientIp: string, input: RegisterInput): Promise<RegisterResult> {
	let res: Response;
	try {
		res = await f(`${BASE}/v1/auth/register`, {
			method: 'POST',
			headers: { 'content-type': 'application/json', 'x-forwarded-for': clientIp },
			body: JSON.stringify(input)
		});
	} catch {
		return { ok: false, code: 'unavailable' };
	}
	if (res.status === 201) return { ok: true };
	if (res.status === 409) return { ok: false, code: 'email_taken' };
	if (res.status === 429) return { ok: false, code: 'rate_limited' };
	if (res.status === 400) {
		const body = await res.json().catch(() => null);
		return { ok: false, code: 'invalid', message: body?.error?.message };
	}
	return { ok: false, code: 'unavailable' };
}

export async function fetchMe(f: typeof fetch, token: string): Promise<Me | null> {
	try {
		const res = await f(`${BASE}/v1/auth/me`, { headers: { cookie: `${SESSION_COOKIE}=${token}` } });
		if (!res.ok) return null;
		const b = await res.json();
		return { id: b.user_id, email: b.email, merchantId: b.merchant_id, permissions: b.permissions };
	} catch {
		return null;
	}
}

export async function logout(f: typeof fetch, token: string): Promise<void> {
	try {
		await f(`${BASE}/v1/auth/logout`, {
			method: 'POST',
			headers: { 'content-type': 'application/json', cookie: `${SESSION_COOKIE}=${token}` },
			body: '{}'
		});
	} catch {
		/* sesi tetap kedaluwarsa di server; cookie lokal dihapus oleh pemanggil */
	}
}

export function setSession(cookies: Cookies, token: string, maxAge: number, secure: boolean) {
	cookies.set(SESSION_COOKIE, token, { path: '/', httpOnly: true, sameSite: 'lax', secure, maxAge });
}
