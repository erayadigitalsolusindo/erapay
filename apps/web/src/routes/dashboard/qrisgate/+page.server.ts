import { fail, redirect } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../../lib/server/auth';
import { call, int, newIdemKey, read, str } from '../../../lib/server/gw';
import { qrSvg } from '../../../lib/server/payments';
import type { Actions, PageServerLoad } from './$types';

export type Topup = {
	id: string;
	amount: number;
	status: 'requested' | 'confirmed' | 'rejected' | 'cancelled';
	method: 'manual' | 'qris';
	qr_payload?: string;
	expires_at?: string;
	created_at: string;
};
export type Movement = { at: string; kind: string; source_id: string; direction: 'debit' | 'credit'; amount: number };

function session(locals: App.Locals, cookies: import('@sveltejs/kit').Cookies) {
	const token = cookies.get(SESSION_COOKIE);
	if (!locals.user || !token) redirect(303, '/login');
	if (!locals.user.merchantId || !locals.user.permissions.includes('merchant:read')) redirect(303, '/dashboard');
	return token;
}

// Semua aturan (nominal, izin, idempotency, saldo) ada di Go; di sini hanya meneruskan.
export type ListenerInfo = { available: boolean; release?: { version_name: string; size_bytes: number; sha256: string; note?: string; created_at: string } };
export type QGAccount = { has_qris: boolean; has_secret: boolean; packages: string[]; ready: boolean; activation_status?: string; webhook_path: string };

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = session(locals, cookies);
	const [overview, topups, history, listener] = await Promise.all([
		read<{ fee: number; balance: number; account: QGAccount }>(fetch, token, '/v1/qrisgate'),
		read<{ data: Topup[] }>(fetch, token, '/v1/qrisgate/topups'),
		read<{ data: Movement[] }>(fetch, token, '/v1/qrisgate/history'),
		read<ListenerInfo>(fetch, token, '/v1/qrisgate/listener')
	]);
	const list = topups?.data ?? [];
	// Hanya top-up QRIS yang masih bisa dibayar punya qr_payload; QR digambar di server.
	const qr: Record<string, string> = {};
	for (const tp of list) if (tp.qr_payload) qr[tp.id] = await qrSvg(tp.qr_payload);
	return {
		overview,
		listener,
		topups: list,
		qr,
		history: history?.data ?? [],
		canRequest: locals.user!.permissions.includes('merchant:write'),
		idem: newIdemKey('qgtopup')
	};
};

export const actions: Actions = {
	topup: async ({ request, locals, cookies, fetch }) => {
		const token = session(locals, cookies);
		const f = await request.formData();
		const amount = int(f, 'amount');
		if (!Number.isFinite(amount) || amount <= 0) return fail(400, { error: true });
		const r = await call(fetch, token, 'POST', '/v1/qrisgate/topups', { amount }, str(f, 'idem'));
		return r.ok ? { sent: true } : fail(r.status === 409 ? 409 : 400, { error: true });
	},
	topupQris: async ({ request, locals, cookies, fetch }) => {
		const token = session(locals, cookies);
		const f = await request.formData();
		const amount = int(f, 'amount');
		if (!Number.isFinite(amount) || amount <= 0) return fail(400, { error: true });
		const r = await call(fetch, token, 'POST', '/v1/qrisgate/topups/qris', { amount }, str(f, 'idem'));
		return r.ok ? { sentQris: true } : fail(r.status === 409 || r.status === 422 ? r.status : 400, { error: true, unavailable: r.status === 422 });
	},
	cancel: async ({ request, locals, cookies, fetch }) => {
		const token = session(locals, cookies);
		const id = str(await request.formData(), 'id');
		const r = await call(fetch, token, 'POST', `/v1/qrisgate/topups/${encodeURIComponent(id)}/cancel`, {});
		return r.ok ? { cancelled: true } : fail(400, { error: true });
	}
};
