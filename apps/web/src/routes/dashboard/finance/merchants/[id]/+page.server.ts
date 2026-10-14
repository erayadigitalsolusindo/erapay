import { redirect } from '@sveltejs/kit';
import { guard, read } from '../../../../../lib/server/gw';
import type { PageServerLoad } from './$types';

// GET /v1/admin/merchants/{id} mengembalikan data diri dengan NIK/NPWP *plaintext* untuk reviewer
// (merchant:review). Halaman ini hanya untuk tampilan itu; daftar tetap memakai nilai ter-masking.
type Merchant = {
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
	created_at: string;
	identity_complete: boolean;
	identity: {
		id_masked?: string;
		id_number?: string;
		place_of_birth?: string;
		date_of_birth?: string;
		gender?: string;
		address?: string;
		npwp_masked?: string;
		npwp?: string;
		business_npwp_masked?: string;
		business_npwp?: string;
		business_nib?: string;
		complete: boolean;
	} | null;
};

export const load: PageServerLoad = async ({ locals, cookies, fetch, params }) => {
	const token = guard(locals, cookies, 'merchant:review', 'platform');
	const m = await read<Merchant>(fetch, token, `/v1/admin/merchants/${encodeURIComponent(params.id)}`);
	if (m === null) throw redirect(303, '/dashboard/finance/merchants');
	return {
		merchant: m,
		canApprove: locals.user!.permissions.includes('merchant:approve'),
		status: 'submitted'
	};
};
