import { fail } from '@sveltejs/kit';
import { guardPlatform, platformApi } from '../../../../lib/server/platform';
import type { Actions, PageServerLoad } from './$types';

export type Row = {
	id: string;
	merchant_name: string;
	merchant_reference: string;
	amount: number;
	currency: string;
	status: string;
	gateway: string;
	created_at: string;
	deletable: boolean;
};

const STATUS = /^(created|pending|succeeded|failed|expired|cancelled|unknown)?$/;

export const load: PageServerLoad = async ({ locals, cookies, fetch, url }) => {
	const token = guardPlatform(locals, cookies);
	const q = url.searchParams.get('q')?.trim() ?? '';
	const status = url.searchParams.get('status') ?? '';
	const qs = new URLSearchParams({ limit: '50' });
	if (q) qs.set('q', q);
	if (STATUS.test(status) && status) qs.set('status', status);
	const r = await platformApi<{ data: Row[] }>(fetch, token, 'GET', `/payments?${qs}`);
	return { q, status, rows: r.ok ? r.value.data ?? [] : [], error: r.ok ? null : r.message };
};

export const actions: Actions = {
	delete: async ({ request, locals, cookies, fetch }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		const id = String(f.get('id') ?? '');
		const reason = String(f.get('reason') ?? '').trim();
		if (!/^[0-9a-f-]{36}$/i.test(id)) return fail(400, { id, message: 'ID transaksi tidak valid.' });
		if (reason.length < 3) return fail(400, { id, message: 'Alasan wajib diisi (minimal 3 karakter).' });
		const r = await platformApi(fetch, token, 'DELETE', `/payments/${id}`, { reason });
		if (!r.ok) return fail(r.status === 0 ? 503 : r.status, { id, message: r.message });
		return { deleted: id };
	}
};
