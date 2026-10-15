import { fail } from '@sveltejs/kit';
import { call, done, guard, read, str } from '../../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export type AdminPayout = {
	id: string;
	merchant_id: string;
	amount: number;
	status: string;
	dry_run: boolean;
	requested_by: string;
	bank_reference: string | null;
	failure_reason: string | null;
	decision_note: string | null;
	gateway?: string;
	fee_amount?: number;
	created_at: string;
};
export type PendingAccount = { id: string; merchant_id: string; bank_code: string; account_masked: string; account_name: string; created_at: string };
export type Bank = { code: string; name: string };

const STATUSES = ['pending_approval', 'approved', 'submitted', 'unknown', 'paid', 'failed', 'returned', 'cancelled'];

export const load: PageServerLoad = async ({ locals, cookies, fetch, url }) => {
	const status = url.searchParams.get('status') ?? '';
	// Sesi merchant: tampilan baca-saja atas payout miliknya sendiri (endpoint tenant /v1/payouts); tanpa aksi review.
	if (locals.user?.merchantId) {
		const token = guard(locals, cookies, 'payout:read', 'merchant');
		const mine = await read<{ data: Omit<AdminPayout, 'merchant_id' | 'requested_by'>[] }>(fetch, token, '/v1/payouts?limit=100');
		const rows = mine?.data
			? mine.data.filter((r) => !STATUSES.includes(status) || r.status === status).map((r) => ({ ...r, merchant_id: locals.user!.merchantId, requested_by: '' }))
			: null;
		const banks = await read<{ data: Bank[] }>(fetch, token, '/v1/banks');
		return { tenant: true, payouts: rows, accounts: [] as PendingAccount[], banks: banks?.data ?? [], status, me: locals.user.id, canVerify: false, canApprove: false, canExecute: false };
	}
	const token = guard(locals, cookies, 'payout:review', 'platform');
	const q = STATUSES.includes(status) ? `&status=${status}` : '';
	const p = locals.user!.permissions;
	const [payouts, accounts, banks] = await Promise.all([
		read<{ data: AdminPayout[] }>(fetch, token, `/v1/admin/payouts?limit=100${q}`),
		read<{ data: PendingAccount[] }>(fetch, token, '/v1/admin/bank-accounts?status=pending_verification'),
		read<{ data: Bank[] }>(fetch, token, '/v1/banks')
	]);
	return {
		tenant: false,
		payouts: payouts?.data ?? null,
		accounts: accounts?.data ?? null,
		banks: banks?.data ?? [],
		status,
		me: locals.user!.id,
		canVerify: p.includes('bank_account:verify'),
		canApprove: p.includes('payout:approve'),
		canExecute: p.includes('payout:execute')
	};
};

const act = (perm: string, path: (id: string) => string, body?: (f: FormData) => Record<string, unknown>) =>
	async ({ request, cookies, fetch, locals }: Parameters<Actions[string]>[0]) => {
		const token = guard(locals, cookies, perm, 'platform');
		const f = await request.formData();
		const id = str(f, 'id');
		return done(await call(fetch, token, 'POST', path(encodeURIComponent(id)), body?.(f)));
	};

export const actions: Actions = {
	verify: act('bank_account:verify', (id) => `/v1/admin/bank-accounts/${id}/verify`),
	approve: act('payout:approve', (id) => `/v1/admin/payouts/${id}/approve`),
	reject: act('payout:approve', (id) => `/v1/admin/payouts/${id}/reject`, (f) => ({ note: str(f, 'note') })),
	markPaid: act('payout:execute', (id) => `/v1/admin/payouts/${id}/mark-paid`, (f) => ({ bank_reference: str(f, 'bank_reference') })),
	markFailed: act('payout:execute', (id) => `/v1/admin/payouts/${id}/mark-failed`, (f) => ({ reason: str(f, 'reason') })),
	execute: act('payout:execute', (id) => `/v1/admin/payouts/${id}/execute`),
	checkStatus: act('payout:execute', (id) => `/v1/admin/payouts/${id}/check-status`),
	reveal: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'payout:execute', 'platform');
		const id = str(await request.formData(), 'id');
		const r = await call<{ bank_code: string; account_number: string; account_name: string }>(fetch, token, 'GET', `/v1/admin/payouts/${encodeURIComponent(id)}/destination`);
		// Ditampilkan sekali di respons ini (dan dicatat di audit log oleh Go); tidak disimpan di server web.
		return r.ok ? { dest: { id, ...r.value } } : fail(400, { message: r.message });
	}
};
