import { dev } from '$app/env';
import { fail, redirect } from '@sveltejs/kit';
import { platformLogin, platformVerifyMfa, setPlatformSession } from '../../../../lib/server/platform';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = ({ locals }) => {
	if (locals.platformUser) redirect(303, '/platform');
};

const MESSAGES: Record<string, { text: string; status: number }> = {
	invalid: { text: 'Email atau kata sandi salah.', status: 401 },
	rate_limited: { text: 'Terlalu banyak percobaan. Coba lagi nanti.', status: 429 },
	invalid_input: { text: 'Kode MFA tidak valid.', status: 400 },
	unavailable: { text: 'Layanan panel sedang tidak tersedia. Coba lagi nanti.', status: 503 }
};

export const actions: Actions = {
	default: async ({ request, fetch, cookies, getClientAddress }) => {
		const form = await request.formData();
		const email = String(form.get('email') ?? '').trim();
		const password = String(form.get('password') ?? '');
		const challengeId = String(form.get('challenge_id') ?? '').trim();
		const code = String(form.get('mfa_code') ?? '').trim();

		if (challengeId && code) {
			const r = await platformVerifyMfa(fetch, getClientAddress(), email, password, code, challengeId);
			if (!r.ok) {
				const m = MESSAGES[r.code] ?? MESSAGES.invalid;
				return fail(m.status, { email, challenge_id: challengeId, mfa: true, error: r.message ?? m.text });
			}
			if (r.ok !== true) return fail(500, { email, challenge_id: challengeId, mfa: true, error: 'Layanan panel sedang tidak tersedia.' });
			setPlatformSession(cookies, r.token, r.maxAge, !dev);
			redirect(303, '/platform');
		}

		if (!email || !password) return fail(400, { email, error: 'Email dan kata sandi wajib diisi.' });

		const r = await platformLogin(fetch, getClientAddress(), email, password);
		if (!r.ok) {
			const m = MESSAGES[r.code] ?? MESSAGES.invalid;
			return fail(m.status, { email, error: r.message ?? m.text });
		}
		if (r.ok === 'mfa') return { email: r.email, challenge_id: r.challengeId, mfa: true, error: null };
		setPlatformSession(cookies, r.token, r.maxAge, !dev);
		redirect(303, '/platform');
	}
};
