import { fail } from '@sveltejs/kit';
import { call, done, guard, int, read, str } from '../../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export type Unsettled = {
	payment_id: string;
	merchant_id: string;
	gateway: string;
	gateway_reference: string;
	currency: string;
	gross: number;
	mdr_fee: number;
	expected_net: number;
	final_at: string;
};
export type Batch = { id: string; gateway: string; batch_ref: string; currency: string; settlement_date: string; reported_net: number; expected_net: number; payment_count: number; created_at: string };
export type Exception = { id: string; gateway: string; reference: string; details: { reported_net: number; expected_net: number; difference: number; payment_count: number }; status: string; created_at: string };

export const load: PageServerLoad = async ({ locals, cookies, fetch, url }) => {
	// Dinamis menurut scope sesi: merchant melihat hanya pembayarannya sendiri (/v1/settlement/unsettled),
	// staf platform melihat lintas tenant beserta batch & exception rekonsiliasi.
	if (locals.user?.merchantId) {
		const token = guard(locals, cookies, 'settlement:read', 'merchant');
		const mine = await read<{ data: Unsettled[] }>(fetch, token, '/v1/settlement/unsettled?limit=200');
		return { tenant: true, unsettled: mine?.data ?? null, batches: [] as Batch[], exceptions: [] as Exception[], canManage: false, gateway: '' };
	}
	const token = guard(locals, cookies, 'settlement:read', 'platform');
	const gateway = (url.searchParams.get('gateway') ?? '').replace(/[^a-z0-9_-]/gi, '').slice(0, 32);
	const q = gateway ? `&gateway=${gateway}` : '';
	const [unsettled, batches, exceptions] = await Promise.all([
		read<{ data: Unsettled[] }>(fetch, token, `/v1/admin/unsettled-payments?limit=200${q}`),
		read<{ data: Batch[] }>(fetch, token, '/v1/admin/settlement-batches?limit=30'),
		read<{ data: Exception[] }>(fetch, token, '/v1/admin/reconciliation-exceptions?status=open&limit=50')
	]);
	return {
		unsettled: unsettled?.data ?? null,
		batches: batches?.data ?? null,
		exceptions: exceptions?.data ?? null,
		canManage: locals.user!.permissions.includes('settlement:manage'),
		gateway
	};
};

export const actions: Actions = {
	record: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'settlement:manage', 'platform');
		const f = await request.formData();
		const ids = f.getAll('payment_ids').map(String);
		const net = int(f, 'reported_net');
		if (ids.length === 0) return fail(400, { message: 'Pilih minimal satu pembayaran.' });
		if (!Number.isSafeInteger(net) || net <= 0) return fail(400, { message: 'Net menurut laporan harus berupa angka rupiah penuh.' });
		const r = await call<{ payment_count: number; expected_net: number }>(fetch, token, 'POST', '/v1/admin/settlement-batches', {
			gateway: str(f, 'gateway'),
			batch_ref: str(f, 'batch_ref'),
			currency: str(f, 'currency') || 'IDR',
			settlement_date: str(f, 'settlement_date'),
			reported_net: net,
			payment_ids: ids,
			note: str(f, 'note') || undefined
		});
		if (r.ok) return { booked: r.value };
		if (r.status === 422 && r.body?.expected_net !== undefined) {
			return fail(422, { mismatch: { reported: r.body.reported_net, expected: r.body.expected_net, diff: r.body.difference } });
		}
		return fail(400, { message: r.message });
	},
	resolve: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'settlement:manage', 'platform');
		const f = await request.formData();
		const note = str(f, 'note');
		if (note.length < 3) return fail(400, { message: 'Catatan penyelesaian wajib diisi.' });
		return done(await call(fetch, token, 'POST', `/v1/admin/reconciliation-exceptions/${encodeURIComponent(str(f, 'id'))}/resolve`, { note }));
	}
};
