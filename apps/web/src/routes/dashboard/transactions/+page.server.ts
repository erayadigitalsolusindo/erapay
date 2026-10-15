import { fail } from '@sveltejs/kit';
import { guard, call, done, read } from '../../../lib/server/gw';
import { getPayment, qrSvg, type Payment } from '../../../lib/server/payments';
import type { Actions, PageServerLoad } from './$types';

const STATUSES = ['created', 'pending', 'succeeded', 'failed', 'expired', 'cancelled', 'unknown'];
const DATE = /^\d{4}-\d{2}-\d{2}$/;
const CURSOR = /^[A-Za-z0-9_-]{1,200}$/;

// Filter dibersihkan di sini hanya untuk UX; Go memvalidasi ulang semuanya.
export const load: PageServerLoad = async ({ locals, cookies, fetch, url }) => {
	const token = guard(locals, cookies, 'payment:read', 'merchant');
	const p = url.searchParams;
	const status = STATUSES.includes(p.get('status') ?? '') ? p.get('status')! : '';
	const ref = (p.get('ref') ?? '').trim().slice(0, 64);
	const from = DATE.test(p.get('from') ?? '') ? p.get('from')! : '';
	const to = DATE.test(p.get('to') ?? '') ? p.get('to')! : '';
	const cursor = CURSOR.test(p.get('cursor') ?? '') ? p.get('cursor')! : '';

	const q = new URLSearchParams({ limit: '25' });
	if (status) q.set('status', status);
	if (ref) q.set('merchant_reference', ref);
	if (from) q.set('from', from);
	if (to) q.set('to', to);
	if (cursor) q.set('cursor', cursor);
	const r = await read<{ data: Payment[]; next_cursor?: string }>(fetch, token, `/v1/payments?${q}`);
	return {
		payments: r?.data ?? null,
		next: r?.next_cursor ?? null,
		filters: { status, ref, from, to },
		paged: !!cursor,
		statuses: STATUSES
	};
};

// Cek status (inquiry): tombol per baris meminta ulang status terbaru dari gateway
// (DOKU) untuk transaksi pending/unknown. Keputusan status tetap di Go (PRD §0 #13).
export const actions: Actions = {
	// QR tidak ada di daftar (hanya di detail); diambil per permintaan dan hanya ada selama pending.
	qr: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'payment:read', 'merchant');
		const id = String((await request.formData()).get('id') ?? '').trim();
		const p = id ? await getPayment(fetch, token, id) : null;
		if (!p?.qr_code) return fail(404, { qrFail: true });
		return { qr: await qrSvg(p.qr_code.value), qrRef: p.merchant_reference, qrAmount: p.amount, qrExpires: p.expires_at };
	},
	inquiry: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'payment:read', 'merchant');
		const f = await request.formData();
		const id = String(f.get('id') ?? '').trim();
		if (!id) return fail(400, { message: 'Transaksi tidak dikenal.' });
		const r = await call(fetch, token, 'POST', `/v1/payments/${encodeURIComponent(id)}/inquiry`);
		return done(r);
	}
};
