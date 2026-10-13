// Pemanggilan endpoint auth di Go API. Semua keputusan keamanan ada di Go (PRD §0 #13);
// di sini hanya meneruskan kredensial dan cookie sesi.
import type { Cookies } from '@sveltejs/kit';

const BASE = process.env.ERAPAY_API_URL ?? 'http://localhost:8080';
export const SESSION_COOKIE = 'erapay_session';

export type Me = { id: string; email: string; merchantId: string; emailVerified: boolean; permissions: string[] };

export type LoginResult =
	| { ok: true; token: string; maxAge: number }
	| { ok: 'mfa'; challengeId: string; email: string }
	| {
			ok: false;
			code: 'invalid' | 'rate_limited' | 'merchant_required' | 'invalid_input' | 'unavailable';
			message?: string;
			retryAfter?: number; // detik sampai boleh mencoba lagi (rate_limited)
			attemptsLeft?: number; // sisa kesempatan sebelum dikunci (invalid)
		  };

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
	if (res.status === 429) return { ok: false, code: 'rate_limited', retryAfter: Number(res.headers.get('retry-after')) || undefined };
	if (res.status === 401) {
		const left = res.headers.get('x-attempts-left');
		return { ok: false, code: 'invalid', attemptsLeft: left === null ? undefined : Number(left) };
	}
	if (res.status === 400) {
		const body = await res.json().catch(() => null);
		return { ok: false, code: /merchant_id/.test(body?.error?.message ?? '') ? 'merchant_required' : 'invalid_input' };
	}
	if (!res.ok) return { ok: false, code: 'unavailable' };
	const body = await res.json().catch(() => null);
	if (body?.mfa_required === true && body?.challenge_id) {
		return { ok: 'mfa', challengeId: body.challenge_id, email };
	}
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
type Code = 'rate_limited' | 'invalid' | 'unavailable';
export type RegisterResult = { ok: true } | { ok: false; code: Code; message?: string };

/** POST publik ke Go API (tanpa sesi). Mengembalikan status + badan JSON, atau null bila jaringan gagal. */
async function publicPost(f: typeof fetch, path: string, clientIp: string, body: unknown) {
	try {
		const res = await f(`${BASE}${path}`, {
			method: 'POST',
			headers: { 'content-type': 'application/json', 'x-forwarded-for': clientIp },
			body: JSON.stringify(body)
		});
		return { status: res.status, body: await res.json().catch(() => null) };
	} catch {
		return null;
	}
}

const fail = (r: { status: number; body: any } | null): { ok: false; code: Code; message?: string } =>
	!r ? { ok: false, code: 'unavailable' }
	: r.status === 429 ? { ok: false, code: 'rate_limited' }
	: r.status === 400 ? { ok: false, code: 'invalid', message: r.body?.error?.message }
	: { ok: false, code: 'unavailable' };

/**
 * Daftar merchant mandiri. Tidak membuat sesi. Jawaban Go sama untuk email baru maupun yang sudah terdaftar
 * (202), jadi halaman ini tidak boleh menyiratkan salah satunya.
 */
export async function register(f: typeof fetch, clientIp: string, input: RegisterInput): Promise<RegisterResult> {
	const r = await publicPost(f, '/v1/auth/register', clientIp, input);
	return r?.status === 202 ? { ok: true } : fail(r);
}

/** Lupa sandi dan kirim ulang verifikasi: selalu 202 untuk alamat apa pun. */
export async function requestEmail(f: typeof fetch, clientIp: string, kind: 'forgot-password' | 'resend-verification', email: string): Promise<RegisterResult> {
	const r = await publicPost(f, `/v1/auth/${kind}`, clientIp, { email });
	return r?.status === 202 ? { ok: true } : fail(r);
}

export type TokenResult = { ok: true } | { ok: false; code: 'invalid_token' | 'weak' | 'unavailable'; message?: string };

async function useToken(f: typeof fetch, path: string, clientIp: string, body: unknown): Promise<TokenResult> {
	const r = await publicPost(f, path, clientIp, body);
	if (r?.status === 200) return { ok: true };
	if (r?.status === 400 && r.body?.error?.code === 'INVALID_TOKEN') return { ok: false, code: 'invalid_token' };
	if (r?.status === 400) return { ok: false, code: 'weak', message: r.body?.error?.message };
	return { ok: false, code: 'unavailable' };
}

export const verifyEmail = (f: typeof fetch, clientIp: string, token: string) => useToken(f, '/v1/auth/verify-email', clientIp, { token });
export const resetPassword = (f: typeof fetch, clientIp: string, token: string, password: string) =>
	useToken(f, '/v1/auth/reset-password', clientIp, { token, password });

export async function fetchMe(f: typeof fetch, token: string): Promise<Me | null> {
	try {
		const res = await f(`${BASE}/v1/auth/me`, { headers: { cookie: `${SESSION_COOKIE}=${token}` } });
		if (!res.ok) return null;
		const b = await res.json();
		return { id: b.user_id, email: b.email, merchantId: b.merchant_id, emailVerified: b.email_verified === true, permissions: b.permissions };
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

export async function verifyMfa(
	f: typeof fetch,
	clientIp: string,
	email: string,
	password: string,
	merchantId: string | undefined,
	code: string,
	challengeId: string
): Promise<LoginResult> {
	let res: Response;
	try {
		res = await f(`${BASE}/v1/auth/mfa/verify`, {
			method: 'POST',
			headers: { 'content-type': 'application/json', 'x-forwarded-for': clientIp },
			body: JSON.stringify({
				email,
				password,
				merchant_id: merchantId || undefined,
				code,
				challenge_id: challengeId
			})
		});
	} catch {
		return { ok: false, code: 'unavailable' };
	}
	if (res.status === 429) return { ok: false, code: 'rate_limited', retryAfter: Number(res.headers.get('retry-after')) || undefined };
	if (res.status === 401) {
		const body = await res.json().catch(() => null);
		const left = res.headers.get('x-attempts-left');
		return {
			ok: false,
			code: 'invalid',
			message: body?.error?.message,
			attemptsLeft: left === null ? undefined : Number(left)
		};
	}
	if (res.status === 400) {
		const body = await res.json().catch(() => null);
		return { ok: false, code: 'invalid_input', message: body?.error?.message };
	}
	if (!res.ok) return { ok: false, code: 'unavailable' };
	for (const line of res.headers.getSetCookie()) {
		const m = line.match(new RegExp(`^${SESSION_COOKIE}=([^;]+)`));
		if (m) {
			const age = line.match(/Max-Age=(\\d+)/i);
			return { ok: true, token: m[1], maxAge: age ? Number(age[1]) : 3600 };
		}
	}
	return { ok: false, code: 'unavailable' };
}
