import { fail, redirect } from '@sveltejs/kit';
import { resetPassword } from '../../../lib/server/auth';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = ({ locals, url }) => {
	if (locals.user) redirect(303, '/dashboard');
	return { token: url.searchParams.get('token') ?? '' };
};

export const actions: Actions = {
	default: async ({ request, fetch, getClientAddress }) => {
		const f = await request.formData();
		const token = String(f.get('token') ?? '').trim();
		const password = String(f.get('password') ?? '');
		if (password !== String(f.get('password_confirm') ?? '')) return fail(400, { done: false as const, expired: false as const, error: 'Kata sandi tidak sama.' });
		const r = await resetPassword(fetch, getClientAddress(), token, password);
		if (r.ok) return { done: true as const };
		if (r.code === 'invalid_token') return fail(400, { done: false as const, expired: true as const, error: 'Tautan tidak valid, sudah kedaluwarsa, atau sudah dipakai.' });
		if (r.code === 'weak') return fail(400, { done: false as const, expired: false as const, error: 'Kata sandi harus 12 sampai 128 karakter.' });
		return fail(503, { done: false as const, expired: false as const, error: 'Layanan sedang tidak tersedia. Coba lagi sebentar lagi.' });
	}
};
