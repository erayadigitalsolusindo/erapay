import { fail, redirect } from '@sveltejs/kit';
import { register } from '../../../lib/server/auth';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = ({ locals }) => {
	if (locals.user) redirect(303, '/dashboard');
};

const MESSAGES = {
	email_taken: 'Email ini sudah terdaftar. Silakan masuk, atau gunakan email lain.',
	rate_limited: 'Terlalu banyak pendaftaran dari jaringan ini. Coba lagi dalam beberapa waktu.',
	unavailable: 'Layanan sedang tidak tersedia. Coba lagi sebentar lagi.',
	invalid: 'Data pendaftaran belum valid. Periksa kembali isian Anda.'
} as const;

// Pendaftaran membuat akun merchant berstatus "submitted"; tim EraPay menyetujuinya (dan menetapkan tarif)
// sebelum pembayaran bisa dipakai. Semua aturan ada di Go; di sini hanya membentuk permintaan dan pesan.
export const actions: Actions = {
	default: async ({ request, fetch, getClientAddress }) => {
		const f = await request.formData();
		const s = (k: string) => String(f.get(k) ?? '').trim();
		const name = s('name');
		const email = s('email');
		const businessName = s('business_name');
		const keep = { name, email, business_name: businessName, success: false as const };

		const password = String(f.get('password') ?? '');
		if (password !== String(f.get('password_confirm') ?? '')) return fail(400, { ...keep, error: 'Kata sandi tidak sama.' });

		const r = await register(fetch, getClientAddress(), {
			account_type: s('jenis') === 'usaha' ? 'business' : 'individual',
			name,
			business_name: businessName,
			email,
			phone: s('phone'),
			password,
			accept_terms: f.get('agree') === 'on'
		});
		if (!r.ok) {
			// Pesan validasi dari Go (bahasa Inggris, aman ditampilkan) ikut disertakan agar pengguna tahu bagian mana.
			const detail = r.code === 'invalid' && r.message ? ` (${r.message})` : '';
			return fail(r.code === 'rate_limited' ? 429 : r.code === 'unavailable' ? 503 : r.code === 'email_taken' ? 409 : 400, {
				...keep,
				error: MESSAGES[r.code] + detail
			});
		}
		return { success: true as const, name, email, business_name: businessName };
	}
};
