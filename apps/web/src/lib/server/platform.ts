// Klien panel platform DEWA di Go API (/v1/platform/*). Panel ini khusus DEWA: cookie
// erapay_platform_session (Path=/platform), terpisah dari dashboard. Semua keputusan
// keamanan tetap di Go (PRD §0 #13); di sini hanya meneruskan kredensial + cookie.
import type { Cookies } from '@sveltejs/kit';
import { redirect } from '@sveltejs/kit';

const BASE = process.env.ERAPAY_API_URL ?? 'http://localhost:8080';
export const PLATFORM_COOKIE = 'erapay_platform_session';

export type PlatformMe = { id: string; email: string; emailVerified: boolean; permissions: string[] };

export type PlatformLoginResult =
	| { ok: true; token: string; maxAge: number }
	| { ok: 'mfa'; challengeId: string; email: string }
	| {
			ok: false;
			code: 'invalid' | 'rate_limited' | 'invalid_input' | 'unavailable';
			message?: string;
			retryAfter?: number;
			attemptsLeft?: number;
	  };

async function platformPost(f: typeof fetch, clientIp: string, path: string, body: unknown): Promise<Response> {
	return f(`${BASE}${path}`, {
		method: 'POST',
		headers: { 'content-type': 'application/json', 'x-forwarded-for': clientIp },
		body: JSON.stringify(body)
	});
}

function cookieWithToken(line: string, name: string): { token: string; maxAge: number } | null {
	const m = line.match(new RegExp(`^${name}=([^;]+)`));
	if (!m) return null;
	const age = line.match(/Max-Age=(\d+)/i);
	return { token: m[1], maxAge: age ? Number(age[1]) : 3600 };
}

export async function platformLogin(
	f: typeof fetch,
	clientIp: string,
	email: string,
	password: string
): Promise<PlatformLoginResult> {
	let res: Response;
	try {
		res = await platformPost(f, clientIp, '/v1/platform/auth/login', { email, password });
	} catch {
		return { ok: false, code: 'unavailable' };
	}
	if (res.status === 429) return { ok: false, code: 'rate_limited', retryAfter: Number(res.headers.get('retry-after')) || undefined };
	if (res.status === 401) {
		const left = res.headers.get('x-attempts-left');
		const body = await res.json().catch(() => null);
		return {
			ok: false,
			code: 'invalid',
			message: body?.error?.message,
			attemptsLeft: left === null ? undefined : Number(left)
		};
	}
	if (res.status === 400) return { ok: false, code: 'invalid', message: 'Email dan kata sandi wajib diisi.' };
	if (!res.ok) return { ok: false, code: 'unavailable' };
	const body = await res.json().catch(() => null);
	if (body?.mfa_required === true && body?.challenge_id) {
		return { ok: 'mfa', challengeId: body.challenge_id, email };
	}
	for (const line of res.headers.getSetCookie()) {
		const c = cookieWithToken(line, PLATFORM_COOKIE);
		if (c) return { ok: true, token: c.token, maxAge: c.maxAge };
	}
	return { ok: false, code: 'unavailable' };
}

export async function platformVerifyMfa(
	f: typeof fetch,
	clientIp: string,
	email: string,
	password: string,
	code: string,
	challengeId: string
): Promise<PlatformLoginResult> {
	let res: Response;
	try {
		res = await platformPost(f, clientIp, '/v1/platform/auth/mfa/verify', {
			email,
			password,
			code,
			challenge_id: challengeId
		});
	} catch {
		return { ok: false, code: 'unavailable' };
	}
	if (res.status === 429) return { ok: false, code: 'rate_limited', retryAfter: Number(res.headers.get('retry-after')) || undefined };
	if (res.status === 400) {
		const b = await res.json().catch(() => null);
		return { ok: false, code: 'invalid_input', message: b?.error?.message };
	}
	if (res.status === 401) {
		const b = await res.json().catch(() => null);
		return { ok: false, code: 'invalid', message: b?.error?.message ?? 'Kode tidak valid.' };
	}
	if (!res.ok) return { ok: false, code: 'unavailable' };
	for (const line of res.headers.getSetCookie()) {
		const c = cookieWithToken(line, PLATFORM_COOKIE);
		if (c) return { ok: true, token: c.token, maxAge: c.maxAge };
	}
	return { ok: false, code: 'unavailable' };
}

/** Sesi panel → principal. Diperbarui tiap request (role bisa ditarik kapan saja). */
export async function fetchPlatformMe(f: typeof fetch, token: string): Promise<PlatformMe | null> {
	try {
		const res = await f(`${BASE}/v1/platform/auth/me`, { headers: { cookie: `${PLATFORM_COOKIE}=${token}` } });
		if (!res.ok) return null;
		const b = await res.json();
		return { id: b.user_id, email: b.email, emailVerified: b.email_verified === true, permissions: b.permissions };
	} catch {
		return null;
	}
}

export async function platformLogout(f: typeof fetch, token: string): Promise<void> {
	try {
		await f(`${BASE}/v1/platform/auth/logout`, {
			method: 'POST',
			headers: { 'content-type': 'application/json', cookie: `${PLATFORM_COOKIE}=${token}` },
			body: '{}'
		});
	} catch {
		/* sesi tetap kedaluwarsa di server; cookie lokal dihapus oleh pemanggil */
	}
}

export function setPlatformSession(cookies: Cookies, token: string, maxAge: number, secure: boolean) {
	cookies.set(PLATFORM_COOKIE, token, { path: '/platform', httpOnly: true, sameSite: 'lax', secure, maxAge });
}

/** Izin dicek di sini hanya untuk UX (redirect); Go memeriksa ulang setiap panggilan. */
export function guardPlatform(locals: App.Locals, cookies: Cookies): string {
	const u = locals.platformUser;
	const token = cookies.get(PLATFORM_COOKIE);
	if (!u || !token) redirect(303, '/platform/login');
	if (!u.permissions.includes('rbac:manage')) redirect(303, '/platform/login');
	return token;
}

/** Jalur API panel platform (RBAC di /v1/platform/admin/*, bukan /v1/admin/*). */
export const platformApi = <T = unknown>(f: typeof fetch, token: string, method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string, body?: unknown): Promise<PlatformResult<T>> =>
	callPlatform<T>(f, token, method, path, body);

async function callPlatform<T>(f: typeof fetch, token: string, method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string, body?: unknown): Promise<PlatformResult<T>> {
	let res: Response;
	try {
		res = await f(`${BASE}/v1/platform${path}`, {
			method,
			headers: { 'content-type': 'application/json', cookie: `${PLATFORM_COOKIE}=${token}` },
			body: method === 'GET' ? undefined : JSON.stringify(body ?? {})
		});
	} catch {
		return { ok: false, message: 'Layanan tidak dapat dihubungi.', status: 0 };
	}
	if (res.status === 204) return { ok: true, value: undefined as T, status: 204 };
	const j = await res.json().catch(() => null);
	if (res.ok) return { ok: true, value: j as T, status: res.status };
	return { ok: false, message: j?.error?.message ?? 'Permintaan gagal.', status: res.status, body: j };
}

// bentuk akhir untuk file lain
export type PlatformResult<T> = { ok: true; value: T; status: number } | { ok: false; message: string; status: number; body?: any };

export type SetupStatus = { done: boolean; enabled: boolean };
export type SetupResult = { ok: true } | { ok: false; message: string; status: number };

/** Status bootstrap panel (apakah DEWA sudah ada / form setup layak ditampilkan). */
export async function platformSetupStatus(f: typeof fetch): Promise<SetupStatus | null> {
	try {
		const res = await f(`${BASE}/v1/platform/setup-status`);
		if (!res.ok) return null;
		return (await res.json()) as SetupStatus;
	} catch {
		return null;
	}
}

/** POST /v1/platform/setup — bootstrap DEWA pertama. */
export async function platformSetup(
	f: typeof fetch,
	clientIp: string,
	token: string,
	email: string,
	password: string
): Promise<SetupResult> {
	try {
		const res = await f(`${BASE}/v1/platform/setup`, {
			method: 'POST',
			headers: { 'content-type': 'application/json', 'x-forwarded-for': clientIp },
			body: JSON.stringify({ token, email, password })
		});
		if (res.status === 201) return { ok: true };
		const b = await res.json().catch(() => null);
		return { ok: false, message: b?.error?.message ?? 'Setup gagal.', status: res.status };
	} catch {
		return { ok: false, message: 'Layanan tidak dapat dihubungi.', status: 0 };
	}
}
