import { call, done, guard, read, str } from '../../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export type AdminRefund = {
	id: string;
	merchant_id: string;
	payment_id: string;
	amount: number;
	reason: string;
	status: string;
	requested_by: string;
	bank_reference: string | null;
	failure_reason: string | null;
	decision_note: string | null;
	created_at: string;
};

const STATUSES = ['pending_approval', 'approved', 'refunded', 'failed', 'cancelled'];

export const load: PageServerLoad = async ({ locals, cookies, fetch, url }) => {
	const status = url.searchParams.get('status') ?? '';
	// Sesi merchant: tampilan baca-saja atas refund miliknya sendiri (endpoint tenant /v1/refunds); tanpa aksi review.
	if (locals.user?.merchantId) {
		const token = guard(locals, cookies, 'refund:read', 'merchant');
		const mine = await read<{ data: Omit<AdminRefund, 'merchant_id' | 'requested_by'>[] }>(fetch, token, '/v1/refunds?limit=100');
		const rows = mine?.data
			? mine.data.filter((r) => !STATUSES.includes(status) || r.status === status).map((r) => ({ ...r, merchant_id: locals.user!.merchantId, requested_by: '' }))
			: null;
		return { tenant: true, refunds: rows, status, me: locals.user.id, canApprove: false, canExecute: false };
	}
	const token = guard(locals, cookies, 'refund:review', 'platform');
	const q = STATUSES.includes(status) ? `&status=${status}` : '';
	const p = locals.user!.permissions;
	const refunds = await read<{ data: AdminRefund[] }>(fetch, token, `/v1/admin/refunds?limit=100${q}`);
	return {
		tenant: false,
		refunds: refunds?.data ?? null,
		status,
		me: locals.user!.id,
		canApprove: p.includes('refund:approve'),
		canExecute: p.includes('refund:execute')
	};
};

const act = (perm: string, path: (id: string) => string, body?: (f: FormData) => Record<string, unknown>) =>
	async ({ request, cookies, fetch, locals }: Parameters<Actions[string]>[0]) => {
		const token = guard(locals, cookies, perm, 'platform');
		const f = await request.formData();
		return done(await call(fetch, token, 'POST', path(encodeURIComponent(str(f, 'id'))), body?.(f)));
	};

export const actions: Actions = {
	approve: act('refund:approve', (id) => `/v1/admin/refunds/${id}/approve`),
	reject: act('refund:approve', (id) => `/v1/admin/refunds/${id}/reject`, (f) => ({ note: str(f, 'note') })),
	markRefunded: act('refund:execute', (id) => `/v1/admin/refunds/${id}/mark-refunded`, (f) => ({ bank_reference: str(f, 'bank_reference') })),
	markFailed: act('refund:execute', (id) => `/v1/admin/refunds/${id}/mark-failed`, (f) => ({ reason: str(f, 'reason') }))
};
