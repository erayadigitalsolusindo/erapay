import { fail } from '@sveltejs/kit';
import { verifyEmail } from '../../../lib/server/auth';
import type { Actions, PageServerLoad } from './$types';

// Halaman ini hanya MENAMPILKAN tombol. Token baru dipakai saat tombol ditekan (POST), karena pemindai
// tautan email dan prefetch membuka URL GET otomatis dan akan menghabiskan token sekali pakai.
export const load: PageServerLoad = ({ url }) => ({ token: url.searchParams.get('token') ?? '' });

export const actions: Actions = {
	default: async ({ request, fetch, getClientAddress, locals }) => {
		const token = String((await request.formData()).get('token') ?? '').trim();
		const r = await verifyEmail(fetch, getClientAddress(), token);
		if (r.ok) return { done: true as const, loggedIn: !!locals.user };
		if (r.code === 'invalid_token') return fail(400, { done: false as const, error: 'Tautan tidak valid, sudah kedaluwarsa, atau sudah dipakai. Minta tautan baru dari menu Status akun setelah masuk.' });
		return fail(503, { done: false as const, error: 'Layanan sedang tidak tersedia. Coba lagi sebentar lagi.' });
	}
};
