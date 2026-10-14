import { fail } from '@sveltejs/kit';
import { call, done, guard, int, read, str } from '../../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export type AdminMerchant = {
	id: string;
	legal_name: string;
	display_name: string;
	account_type: 'individual' | 'business' | null;
	owner_name: string | null;
	owner_email: string | null;
	owner_email_verified: boolean;
	contact_phone: string | null;
	status: string;
	review_note: string | null;
	reviewed_at: string | null;
	created_at: string;
	identity_complete: boolean;
};

const STATUSES = ['submitted', 'under_review', 'action_required', 'approved', 'rejected', 'suspended'];

export const load: PageServerLoad = async ({ locals, cookies, fetch, url }) => {
	const token = guard(locals, cookies, 'merchant:review', 'platform');
	const asked = url.searchParams.get('status') ?? '';
	const status = STATUSES.includes(asked) ? asked : 'submitted';
	const merchants = await read<{ data: AdminMerchant[] }>(fetch, token, `/v1/admin/merchants?limit=100&status=${status}`);
	return {
		merchants: merchants?.data ?? null,
		status,
		canApprove: locals.user!.permissions.includes('merchant:approve'),
		canReview: locals.user!.permissions.includes('merchant:review')
	};
};

export const actions: Actions = {
	startReview: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'merchant:review', 'platform');
		const f = await request.formData();
		return done(await call(fetch, token, 'POST', `/v1/admin/merchants/${encodeURIComponent(str(f, 'id'))}/start-review`, {}));
	},
	requestChanges: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'merchant:review', 'platform');
		const f = await request.formData();
		return done(await call(fetch, token, 'POST', `/v1/admin/merchants/${encodeURIComponent(str(f, 'id'))}/request-changes`, { note: str(f, 'note') }));
	},
	// Tarif tidak punya nilai bawaan: keduanya wajib bilangan bulat 0..10000 (basis point, 100 = 1%).
	approve: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'merchant:approve', 'platform');
		const f = await request.formData();
		const mdr = int(f, 'mdr_bps');
		const platform = int(f, 'platform_bps');
		if (![mdr, platform].every((n) => Number.isInteger(n) && n >= 0 && n <= 10000)) {
			return fail(400, { message: 'MDR dan fee platform harus bilangan bulat 0 sampai 10000 (basis point).' });
		}
		return done(await call(fetch, token, 'POST', `/v1/admin/merchants/${encodeURIComponent(str(f, 'id'))}/approve`, { mdr_bps: mdr, platform_bps: platform }));
	},
	reject: async ({ request, cookies, fetch, locals }) => {
		const token = guard(locals, cookies, 'merchant:approve', 'platform');
		const f = await request.formData();
		return done(await call(fetch, token, 'POST', `/v1/admin/merchants/${encodeURIComponent(str(f, 'id'))}/reject`, { note: str(f, 'note') }));
	}
};
