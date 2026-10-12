import { fail, redirect } from '@sveltejs/kit';
import { requestEmail } from '../../../lib/server/auth';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = ({ locals }) => {
	if (locals.user) redirect(303, '/dashboard');
};

export const actions: Actions = {
	default: async ({ request, fetch, getClientAddress }) => {
		const email = String((await request.formData()).get('email') ?? '').trim();
		if (!email) return fail(400, { email, sent: false as const, error: 'Email wajib diisi.' });
		const r = await requestEmail(fetch, getClientAddress(), 'forgot-password', email);
		if (!r.ok) {
			const error = r.code === 'rate_limited' ? 'Terlalu banyak permintaan. Coba lagi nanti.' : r.code === 'invalid' ? 'Masukkan alamat email yang valid.' : 'Layanan sedang tidak tersedia. Coba lagi sebentar lagi.';
			return fail(r.code === 'rate_limited' ? 429 : r.code === 'invalid' ? 400 : 503, { email, sent: false as const, error });
		}
		// Sama untuk alamat terdaftar maupun tidak.
		return { email, sent: true as const };
	}
};
