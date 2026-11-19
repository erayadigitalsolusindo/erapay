import { json } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../../lib/server/auth';
import { read } from '../../../lib/server/gw';
import type { Payment } from '../../../lib/server/payments';
import type { RequestHandler } from './$types';

// Dipoll SuaraKasir: pembayaran QRIS (EraPay) terbaru yang sudah berhasil. Tunai tidak lewat EraPay, jadi tidak ada di sini.
export const GET: RequestHandler = async ({ locals, cookies, fetch }) => {
	const token = cookies.get(SESSION_COOKIE);
	const u = locals.user;
	if (!token || !u?.merchantId || !u.permissions.includes('payment:read')) return json([], { status: 401 });
	const r = await read<{ data: Payment[] }>(fetch, token, '/v1/payments?limit=10&status=succeeded');
	return json((r?.data ?? []).map((p) => ({ id: p.id, amount: p.amount })));
};
