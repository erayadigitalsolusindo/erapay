import { dev } from '$app/env';
import { fail, redirect } from '@sveltejs/kit';
import { login, setSession } from '../../lib/server/auth';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = ({ locals }) => {
	if (locals.user) redirect(303, '/dashboard');
};

const MESSAGES: Record<string, string> = {
	invalid: 'Email atau kata sandi salah.',
	rate_limited: 'Terlalu banyak percobaan. Coba lagi dalam beberapa menit.',
	invalid_input: 'Email dan kata sandi wajib diisi.',
	unavailable: 'Layanan sedang tidak tersedia. Coba lagi sebentar lagi.'
};

export const actions: Actions = {
	default: async ({ request, fetch, cookies, getClientAddress }) => {
		const form = await request.formData();
		const email = String(form.get('email') ?? '').trim();
		const password = String(form.get('password') ?? '');
		const merchantId = String(form.get('merchant_id') ?? '').trim();
		if (!email || !password) return fail(400, { email, error: MESSAGES.invalid_input });

		const r = await login(fetch, getClientAddress(), email, password, merchantId);
		if (!r.ok) {
			if (r.code === 'merchant_required') {
				return fail(400, { email, needMerchant: true, error: 'Akun ini terdaftar di beberapa merchant. Masukkan ID merchant.' });
			}
			return fail(r.code === 'rate_limited' ? 429 : r.code === 'unavailable' ? 503 : 401, {
				email,
				error: MESSAGES[r.code]
			});
		}
		setSession(cookies, r.token, r.maxAge, !dev);
		redirect(303, '/dashboard');
	}
};
