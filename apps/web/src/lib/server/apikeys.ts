// Pemanggilan /v1/api-keys di Go API memakai sesi dashboard. Izin (api_key:manage), scope yang
// boleh, dan pembuatan secret seluruhnya ditentukan Go (PRD §0 #13); di sini hanya meneruskan.
import { SESSION_COOKIE } from './auth';

const BASE = process.env.ERAPAY_API_URL ?? 'http://localhost:8080';

export const SCOPES = [
	{ id: 'payment:create', label: 'Buat pembayaran' },
	{ id: 'payment:read', label: 'Baca pembayaran' },
	{ id: 'report:read', label: 'Baca laporan' }
] as const;

export type ApiKey = {
	id: string;
	name: string;
	environment: string;
	prefix: string;
	scopes: string[];
	created_at: string;
	expires_at: string | null;
	revoked_at: string | null;
	last_used_at: string | null;
};
export type CreatedKey = ApiKey & { secret: string };

const headers = (token: string) => ({ 'content-type': 'application/json', cookie: `${SESSION_COOKIE}=${token}` });

async function call(f: typeof fetch, token: string, path: string, init: RequestInit = {}) {
	try {
		return await f(`${BASE}/v1/api-keys${path}`, { ...init, headers: headers(token) });
	} catch {
		return null;
	}
}

export async function listKeys(f: typeof fetch, token: string): Promise<ApiKey[] | null> {
	const res = await call(f, token, '');
	return res?.ok ? ((await res.json()).data as ApiKey[]) : null;
}

export type Result<T> = { ok: true; value: T } | { ok: false; message: string };

async function result<T>(res: Response | null, fallback: string): Promise<Result<T>> {
	if (!res) return { ok: false, message: 'Layanan tidak dapat dihubungi.' };
	if (res.status === 204) return { ok: true, value: undefined as T };
	const body = await res.json().catch(() => null);
	return res.ok ? { ok: true, value: body as T } : { ok: false, message: body?.error?.message ?? fallback };
}

export async function createKey(f: typeof fetch, token: string, name: string, scopes: string[]) {
	return result<CreatedKey>(await call(f, token, '', { method: 'POST', body: JSON.stringify({ name, scopes }) }), 'Gagal membuat API key.');
}

export async function revokeKey(f: typeof fetch, token: string, id: string) {
	return result<void>(await call(f, token, `/${encodeURIComponent(id)}/revoke`, { method: 'POST', body: '{}' }), 'Gagal mencabut API key.');
}

/** Rotasi: key lama tetap berlaku selama `graceSeconds` agar klien sempat mengganti. */
export async function rotateKey(f: typeof fetch, token: string, id: string, graceSeconds: number) {
	return result<CreatedKey>(
		await call(f, token, `/${encodeURIComponent(id)}/rotate`, { method: 'POST', body: JSON.stringify({ grace_seconds: graceSeconds }) }),
		'Gagal merotasi API key.'
	);
}
