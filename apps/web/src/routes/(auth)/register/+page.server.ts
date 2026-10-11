import { fail, redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = ({ locals }) => {
	if (locals.user) redirect(303, '/dashboard');
};

// Belum ada endpoint pendaftaran di Go API, dan "merchant daftar mandiri atau dibuat tim?"
// masih open question (docs/prd/open-questions.md). Form sudah divalidasi di sini, tetapi
// tidak ada akun yang dibuat. Saat endpoint ada: panggil di sini lalu kembalikan
// { success: true, name, email } agar UI menampilkan layar sukses.
export const actions: Actions = {
	default: async ({ request }) => {
		const f = await request.formData();
		const email = String(f.get('email') ?? '').trim();
		const name = String(f.get('name') ?? '').trim();
		if (!name || !email) return fail(400, { success: false as const, name, email, error: 'Nama dan email wajib diisi.' });
		return fail(503, {
			success: false as const,
			name,
			email,
			error: 'Pendaftaran mandiri belum dibuka. Akun dibuat oleh tim EraPay; hubungi kami untuk mendapatkan akses.'
		});
	}
};
