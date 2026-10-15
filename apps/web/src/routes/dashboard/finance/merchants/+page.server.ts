import { guard, read } from '../../../../lib/server/gw';
import { merchantActions } from '../../../../lib/server/merchantActions';
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

export const actions: Actions = merchantActions;
