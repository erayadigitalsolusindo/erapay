// Klien umum ke Go API untuk halaman payout, refund, webhook, dan finance. Semua aturan uang,
// izin, dan batas ada di Go (PRD §0 #13); di sini hanya meneruskan sesi dan membentuk hasil.
import { fail, redirect, type Cookies } from '@sveltejs/kit';
import { SESSION_COOKIE } from './auth';

const BASE = process.env.ERAPAY_API_URL ?? 'http://localhost:8080';

export type Result<T> = { ok: true; value: T; status: number } | { ok: false; message: string; status: number; body?: any };

/** Panggil Go API sebagai pengguna yang sedang login. Tidak pernah melempar: kegagalan jaringan jadi hasil `ok:false`. */
export async function call<T = unknown>(
	f: typeof fetch,
	token: string,
	method: 'GET' | 'POST',
	path: string,
	body?: unknown,
	idempotencyKey?: string
): Promise<Result<T>> {
	let res: Response;
	try {
		res = await f(`${BASE}${path}`, {
			method,
			headers: {
				'content-type': 'application/json',
				cookie: `${SESSION_COOKIE}=${token}`,
				...(idempotencyKey ? { 'idempotency-key': idempotencyKey } : {})
			},
			body: method === 'POST' ? JSON.stringify(body ?? {}) : undefined
		});
	} catch {
		return { ok: false, message: 'Layanan tidak dapat dihubungi.', status: 0 };
	}
	if (res.status === 204) return { ok: true, value: undefined as T, status: 204 };
	const json = await res.json().catch(() => null);
	if (res.ok) return { ok: true, value: json as T, status: res.status };
	return { ok: false, message: json?.error?.message ?? 'Permintaan gagal.', status: res.status, body: json };
}

/** GET yang hanya mengembalikan data atau null (halaman menampilkan "gagal dimuat"). */
export async function read<T>(f: typeof fetch, token: string, path: string): Promise<T | null> {
	const r = await call<T>(f, token, 'GET', path);
	return r.ok ? r.value : null;
}

/** Izin dicek di sini hanya untuk UX (redirect); Go memeriksa ulang setiap panggilan. */
export function guard(locals: App.Locals, cookies: Cookies, permission: string, scope: 'merchant' | 'platform'): string {
	const u = locals.user;
	const token = cookies.get(SESSION_COOKIE);
	if (!u || !token) redirect(303, '/login');
	const okScope = scope === 'merchant' ? !!u.merchantId : !u.merchantId;
	if (!okScope || !u.permissions.includes(permission)) redirect(303, '/dashboard');
	return token;
}

/** Kunci idempotency per render halaman: submit ganda dari form yang sama menghasilkan satu operasi. */
export const newIdemKey = (prefix: string) => `${prefix}-${crypto.randomUUID()}`;

export const str = (form: FormData, k: string) => String(form.get(k) ?? '').trim();
export const int = (form: FormData, k: string) => {
	const v = str(form, k).replace(/[.\s]/g, '');
	return /^\d{1,15}$/.test(v) ? Number(v) : NaN;
};

/** Ubah hasil panggilan menjadi respons action: sukses → data tambahan, gagal → fail(). */
export function done<T>(r: Result<T>, ok: Record<string, unknown> = {}) {
	return r.ok ? { done: true, ...ok } : fail(r.status === 409 ? 409 : 400, { message: r.message });
}
