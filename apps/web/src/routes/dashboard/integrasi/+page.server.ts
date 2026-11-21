import { fail, redirect } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../../lib/server/auth';
import { SCOPES, createKey, deleteKey, listKeys, revokeKey, rotateKey } from '../../../lib/server/apikeys';
import { API_HOST } from '../../../lib/utils/apiHost';
import { call, done, read, str } from '../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export type QGAccount = {
	has_qris: boolean;
	qris_name?: string;
	qris_payload?: string;
	has_secret: boolean;
	secret_hint?: string;
	packages: string[];
	exact_amount: boolean;
	ready: boolean;
	activation_status?: string;
	webhook_path: string;
};
export type QGEvent = { id: string; received_at: string; package: string; title: string; text: string; amount?: number; status: 'matched' | 'unmatched' | 'ignored'; reason?: string; payment_id?: string };
export type Endpoint = { id: string; url: string; status: string; secret_hint: string; created_at: string; secret?: string };
export type Delivery = {
	id: string;
	endpoint_id: string;
	event_type: string;
	status: string;
	attempts: number;
	next_attempt_at: string | null;
	last_http_status: number | null;
	last_error: string | null;
	redelivery_of: string | null;
	created_at: string;
};

const STATUSES = ['pending', 'succeeded', 'dead', 'cancelled'];
const KEY = 'api_key:manage';
const HOOK = 'webhook:manage';

// Satu halaman dua kemampuan: tiap bagian tetap dijaga izinnya sendiri (Go memeriksa ulang setiap panggilan).
function session(locals: App.Locals, cookies: import('@sveltejs/kit').Cookies, perm?: string) {
	const u = locals.user;
	const token = cookies.get(SESSION_COOKIE);
	if (!u || !token) redirect(303, '/login');
	if (!u.merchantId || !(perm ? u.permissions.includes(perm) : u.permissions.includes(KEY) || u.permissions.includes(HOOK))) redirect(303, '/dashboard');
	return token;
}

export const load: PageServerLoad = async ({ locals, cookies, fetch, url }) => {
	const token = session(locals, cookies);
	const perms = locals.user!.permissions;
	const canKeys = perms.includes(KEY);
	const canHooks = perms.includes(HOOK);
	const status = url.searchParams.get('status') ?? '';
	const q = STATUSES.includes(status) ? `&status=${status}` : '';
	const wanted = url.searchParams.get('tab');
	// Tab QRISGate: akun QRISGate tenant (kunci dan secret milik tenant); hanya ada bila API-nya mendukung dan izin merchant:read ada.
	const canQG = perms.includes('merchant:read');
	const qg = canQG ? await read<{ account: QGAccount }>(fetch, token, '/v1/qrisgate') : null;
	const qgEvents = qg ? await read<{ data: QGEvent[] }>(fetch, token, '/v1/qrisgate/events') : null;
	const tab =
		wanted === 'qrisgate' && qg ? 'qrisgate' : wanted === 'webhooks' && canHooks ? 'webhooks' : wanted === 'keys' && canKeys ? 'keys' : canKeys ? 'keys' : 'webhooks';
	const [keys, endpoints, deliveries] = await Promise.all([
		canKeys ? listKeys(fetch, token) : Promise.resolve(null),
		canHooks ? read<{ data: Endpoint[] }>(fetch, token, '/v1/webhook-endpoints') : Promise.resolve(null),
		canHooks ? read<{ data: Delivery[] }>(fetch, token, `/v1/webhook-deliveries?limit=50${q}`) : Promise.resolve(null)
	]);
	return {
		tab,
		qgAccount: qg?.account ?? null,
		qgEvents: qgEvents?.data ?? [],
		canEditQG: perms.includes('merchant:write'),
		// URL publik API untuk disalin ke dashboard QRISGate; kosong = halaman menampilkan placeholder.
		apiBase: (process.env.ERAPAY_PUBLIC_API_URL || API_HOST).replace(/\/$/, ''),
		canKeys,
		canHooks,
		keys,
		scopes: SCOPES.filter((s) => perms.includes(s.id)),
		endpoints: endpoints?.data ?? null,
		deliveries: deliveries?.data ?? null,
		status
	};
};

export const actions: Actions = {
	// Tes webhook: server menandatangani notifikasi uji dengan secret tersimpan lalu memverifikasinya sendiri.
	qgTest: async ({ cookies, fetch, locals }) => {
		const token = session(locals, cookies, 'merchant:write');
		const r = await call<{ ok: boolean; steps: { name: string; ok: boolean }[] }>(fetch, token, 'POST', '/v1/qrisgate/account/test', {});
		return r.ok ? { qgTest: r.value } : fail(400, { accountError: true });
	},
	// Tiga langkah penyiapan QRISGate, masing-masing berdiri sendiri. Aturan (validasi QRIS, CRC, paket, rahasia) ada di Go.
	qgQris: async ({ request, cookies, fetch, locals }) => {
		const token = session(locals, cookies, 'merchant:write');
		const f = await request.formData();
		const r = await call<{ qris_name: string }>(fetch, token, 'PUT', '/v1/qrisgate/qris', { payload: str(f, 'payload') });
		return r.ok ? { savedQris: r.value.qris_name } : fail(r.status === 422 ? 422 : 400, { accountError: 'qris', unavailable: r.status === 422 });
	},
	qgPackages: async ({ request, cookies, fetch, locals }) => {
		const token = session(locals, cookies, 'merchant:write');
		const f = await request.formData();
		const packages = str(f, 'packages').split(/[\s,]+/).filter(Boolean);
		const r = await call(fetch, token, 'PUT', '/v1/qrisgate/packages', { packages });
		return r.ok ? { savedPackages: true } : fail(r.status === 422 ? 422 : 400, { accountError: 'packages', unavailable: r.status === 422 });
	},
	// Rahasia listener: hanya ada di respons ini; tidak disimpan di server web.
	qgSecret: async ({ cookies, fetch, locals }) => {
		const token = session(locals, cookies, 'merchant:write');
		const r = await call<{ secret: string }>(fetch, token, 'POST', '/v1/qrisgate/listener-secret', {});
		return r.ok ? { newSecret: r.value.secret } : fail(r.status === 422 ? 422 : 400, { accountError: 'secret', unavailable: r.status === 422 });
	},
	keyCreate: async ({ request, cookies, fetch, locals }) => {
		const token = session(locals, cookies, KEY);
		const form = await request.formData();
		const name = String(form.get('name') ?? '').trim();
		const scopes = form.getAll('scopes').map(String);
		if (!name || name.length > 80) return fail(400, { message: 'Nama wajib diisi (maks. 80 karakter).', name });
		if (scopes.length === 0) return fail(400, { message: 'Pilih minimal satu izin.', name });
		const res = await createKey(fetch, token, name, scopes);
		if (!res.ok) return fail(400, { message: res.message, name });
		// Secret hanya ada di respons ini; tidak disimpan di server web.
		return { createdKey: res.value };
	},
	keyRevoke: async ({ request, cookies, fetch, locals }) => {
		const token = session(locals, cookies, KEY);
		const id = String((await request.formData()).get('id') ?? '');
		const res = await revokeKey(fetch, token, id);
		return res.ok ? { revoked: id } : fail(400, { message: res.message });
	},
	keyDelete: async ({ request, cookies, fetch, locals }) => {
		const token = session(locals, cookies, KEY);
		const id = String((await request.formData()).get('id') ?? '');
		const res = await deleteKey(fetch, token, id);
		return res.ok ? { deleted: id } : fail(400, { message: res.message });
	},
	keyRotate: async ({ request, cookies, fetch, locals }) => {
		const token = session(locals, cookies, KEY);
		const f = await request.formData();
		const id = String(f.get('id') ?? '');
		const grace = Math.min(86400, Math.max(0, Number(f.get('grace') ?? 0) || 0));
		const res = await rotateKey(fetch, token, id, grace);
		return res.ok ? { createdKey: res.value } : fail(400, { message: res.message });
	},
	hookCreate: async ({ request, cookies, fetch, locals }) => {
		const token = session(locals, cookies, HOOK);
		const url = str(await request.formData(), 'url');
		if (!url.startsWith('https://')) return fail(400, { message: 'URL harus diawali https://' });
		const r = await call<Endpoint>(fetch, token, 'POST', '/v1/webhook-endpoints', { url });
		return r.ok ? { createdHook: r.value } : fail(400, { message: r.message });
	},
	hookRotate: async ({ request, cookies, fetch, locals }) => {
		const token = session(locals, cookies, HOOK);
		const id = str(await request.formData(), 'id');
		const r = await call<Endpoint>(fetch, token, 'POST', `/v1/webhook-endpoints/${encodeURIComponent(id)}/rotate-secret`);
		return r.ok ? { createdHook: r.value } : fail(400, { message: r.message });
	},
	hookDisable: async ({ request, cookies, fetch, locals }) => {
		const token = session(locals, cookies, HOOK);
		const id = str(await request.formData(), 'id');
		return done(await call(fetch, token, 'POST', `/v1/webhook-endpoints/${encodeURIComponent(id)}/disable`));
	},
	hookRedeliver: async ({ request, cookies, fetch, locals }) => {
		const token = session(locals, cookies, HOOK);
		const id = str(await request.formData(), 'id');
		return done(await call(fetch, token, 'POST', `/v1/webhook-deliveries/${encodeURIComponent(id)}/redeliver`));
	}
};
