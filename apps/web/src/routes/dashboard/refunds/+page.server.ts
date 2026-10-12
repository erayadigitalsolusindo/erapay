import { call, done, guard, read, str } from '../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export type Refund = {
	id: string;
	payment_id: string;
	amount: number;
	reason: string;
	status: string;
	decision_note: string | null;
	bank_reference: string | null;
	failure_reason: string | null;
	created_at: string;
};

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = guard(locals, cookies, 'refund:read', 'merchant');
	const r = await read<{ data: Refund[] }>(fetch, token, '/v1/refunds?limit=100');
	return { refunds: r?.data ?? null, canRequest: locals.user!.permissions.includes('refund:request') };
};

export const actions: Actions = {
	cancel: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'refund:request', 'merchant');
		const id = str(await request.formData(), 'id');
		return done(await call(fetch, token, 'POST', `/v1/refunds/${encodeURIComponent(id)}/cancel`));
	}
};
