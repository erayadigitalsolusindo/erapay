import { fail } from '@sveltejs/kit';
import { requestEmail } from '../../../lib/server/auth';
import { call, guard, read, str } from '../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

type Profile = {
	id: string;
	legal_name: string;
	display_name: string;
	account_type?: 'individual' | 'business' | null;
	onboarding_status: string;
	review_note: string | null;
};

// Data diri dari GET /v1/merchant/identity: NIK/NPWP ter-masking; field lainnya apa adanya.
type Identity = {
	id_masked?: string;
	place_of_birth?: string;
	date_of_birth?: string;
	gender?: string;
	address?: string;
	npwp_masked?: string;
	business_npwp_masked?: string;
	business_nib?: string;
	complete: boolean;
};

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = guard(locals, cookies, 'merchant:write', 'merchant');
	return {
		profile: await read<Profile>(fetch, token, '/v1/merchant/profile'),
		email: locals.user!.email,
		emailVerified: locals.user!.emailVerified,
		identity: await read<Identity>(fetch, token, '/v1/merchant/identity')
	};
};

export const actions: Actions = {
	// Alamat diambil dari sesi, bukan dari form: tombol ini hanya untuk akun yang sedang login.
	resend: async ({ locals, cookies, fetch, getClientAddress }) => {
		guard(locals, cookies, 'merchant:write', 'merchant');
		const r = await requestEmail(fetch, getClientAddress(), 'resend-verification', locals.user!.email);
		return r.ok ? { resent: true } : fail(r.code === 'rate_limited' ? 429 : 503, { message: r.code === 'rate_limited' ? 'Terlalu banyak permintaan. Coba lagi nanti.' : 'Layanan sedang tidak tersedia.' });
	},
	resubmit: async ({ request, locals, cookies, fetch }) => {
		const token = guard(locals, cookies, 'merchant:write', 'merchant');
		const f = await request.formData();
		const body: Record<string, string> = {};
		if (str(f, 'legal_name')) body.legal_name = str(f, 'legal_name');
		if (str(f, 'contact_phone')) body.contact_phone = str(f, 'contact_phone');
		const r = await call(fetch, token, 'POST', '/v1/merchant/resubmit', body);
		return r.ok ? { submitted: true } : fail(r.status === 409 ? 409 : 400, { message: r.message });
	},
	// Lengkapi/perbaiki data diri selama proses verifikasi. Aturan (wajib/tidak, format) ada di Go.
	identity: async ({ request, locals, cookies, fetch }) => {
		const token = guard(locals, cookies, 'merchant:write', 'merchant');
		const f = await request.formData();
		const s = (k: string) => String(f.get(k) ?? '').trim();
		const identity: Record<string, string> = {
			id_number: s('nik'),
			place_of_birth: s('pob'),
			date_of_birth: s('dob'),
			gender: s('gender'),
			address: s('address')
		};
		if (s('npwp')) identity.npwp = s('npwp');
		if (s('biz_npwp')) identity.business_npwp = s('biz_npwp');
		if (s('nib')) identity.business_nib = s('nib');
		const r = await call<{ complete: boolean }>(fetch, token, 'POST', '/v1/merchant/identity', { identity });
		if (r.ok) return { idSaved: true, identityComplete: r.value.complete };
		return fail(r.status === 409 ? 409 : 400, { message: r.message });
	}
};
