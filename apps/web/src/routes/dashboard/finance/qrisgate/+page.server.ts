import { call, guard, read, str } from '../../../../lib/server/gw';
import { fail } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';

export type AdminTopup = { id: string; merchant_id: string; amount: number; status: string; bank_reference?: string; note?: string; created_at: string };

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = guard(locals, cookies, 'settlement:read', 'platform');
	const r = await read<{ data: AdminTopup[] }>(fetch, token, '/v1/admin/qrisgate-topups');
	return { topups: r?.data ?? null, canManage: locals.user!.permissions.includes('settlement:manage'), me: locals.user!.id };
};

// Aturan (pembuat ≠ penyetuju, sekali saja, ledger) ada di Go; di sini hanya meneruskan.
function decide(path: 'confirm' | 'reject'): import('./$types').Action {
	return async ({ request, locals, cookies, fetch }) => {
		const token = guard(locals, cookies, 'settlement:manage', 'platform');
		const f = await request.formData();
		const body = path === 'confirm' ? { bank_reference: str(f, 'ref') } : { note: str(f, 'note') };
		const r = await call(fetch, token, 'POST', `/v1/admin/qrisgate-topups/${encodeURIComponent(str(f, 'id'))}/${path}`, body);
		return r.ok ? { done: true } : fail(r.status === 409 || r.status === 403 ? r.status : 400, { message: r.message });
	};
}

export const actions: Actions = { confirm: decide('confirm'), reject: decide('reject') };
