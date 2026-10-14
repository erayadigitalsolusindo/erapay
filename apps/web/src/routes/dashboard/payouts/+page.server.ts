import { done, guard, int, newIdemKey, read, call, str } from '../../../lib/server/gw';
import { fail } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';

export type Balance = { currency: string; payable: number; pending: number; available: number; in_payout: number; on_hold: number };
export type BankAccount = { id: string; bank_code: string; account_masked: string; account_name: string; status: string; created_at: string };
export type Payout = {
	id: string;
	bank_account_id: string;
	amount: number;
	status: string;
	dry_run: boolean;
	bank_reference: string | null;
	failure_reason: string | null;
	decision_note: string | null;
	created_at: string;
};

export type Bank = { code: string; name: string };

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = guard(locals, cookies, 'payout:read', 'merchant');
	const perms = locals.user!.permissions;
	const canManage = perms.includes('bank_account:manage');
	const [balance, payouts, accounts, banks] = await Promise.all([
		read<Balance>(fetch, token, '/v1/balance'),
		read<{ data: Payout[] }>(fetch, token, '/v1/payouts?limit=100'),
		canManage ? read<{ data: BankAccount[] }>(fetch, token, '/v1/bank-accounts') : Promise.resolve({ data: [] as BankAccount[] }),
		read<{ data: Bank[] }>(fetch, token, '/v1/banks')
	]);
	return {
		balance,
		payouts: payouts?.data ?? null,
		accounts: accounts?.data ?? null,
		banks: banks?.data ?? [],
		canManage,
		canRequest: perms.includes('payout:request'),
		idem: newIdemKey('po') // satu kunci per tampilan halaman: submit ganda = satu payout
	};
};

export const actions: Actions = {
	addAccount: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'bank_account:manage', 'merchant');
		const f = await request.formData();
		const r = await call(fetch, token, 'POST', '/v1/bank-accounts', {
			bank_code: str(f, 'bank_code'),
			account_number: str(f, 'account_number'),
			account_name: str(f, 'account_name')
		});
		return done(r);
	},
	disableAccount: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'bank_account:manage', 'merchant');
		const id = str(await request.formData(), 'id');
		return done(await call(fetch, token, 'POST', `/v1/bank-accounts/${encodeURIComponent(id)}/disable`));
	},
	request: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'payout:request', 'merchant');
		const f = await request.formData();
		const amount = int(f, 'amount');
		if (!Number.isSafeInteger(amount) || amount <= 0) return fail(400, { message: 'Nominal harus berupa angka rupiah penuh.' });
		const r = await call(fetch, token, 'POST', '/v1/payouts', { bank_account_id: str(f, 'bank_account_id'), amount, currency: 'IDR' }, str(f, 'idem'));
		return done(r);
	},
	cancel: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'payout:request', 'merchant');
		const id = str(await request.formData(), 'id');
		return done(await call(fetch, token, 'POST', `/v1/payouts/${encodeURIComponent(id)}/cancel`));
	}
};

