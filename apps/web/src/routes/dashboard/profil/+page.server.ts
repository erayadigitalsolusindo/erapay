import { fail, redirect } from '@sveltejs/kit';
import QRCode from 'qrcode';
import { SESSION_COOKIE } from '../../../lib/server/auth';
import { call, read, str } from '../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export type Profile = {
	user_id: string;
	email: string;
	full_name: string;
	phone: string;
	email_verified: boolean;
	mfa_enabled: boolean;
	mfa_enrolled_at: string | null;
	recovery_codes_left: number;
	created_at: string;
};

function token(cookies: { get(n: string): string | undefined }, locals: App.Locals) {
	const t = cookies.get(SESSION_COOKIE);
	if (!locals.user || !t) redirect(303, '/login');
	// Sesi "masuk sebagai tenant" memakai akun DEWA: profil/2FA-nya tidak boleh disentuh dari sini.
	if (locals.user.impersonating) redirect(303, '/dashboard');
	return t;
}

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const t = token(cookies, locals);
	return { profile: await read<Profile>(fetch, t, '/v1/profile') };
};

export const actions: Actions = {
	saveProfile: async ({ request, cookies, locals, fetch }) => {
		const t = token(cookies, locals);
		const f = await request.formData();
		const r = await call<Profile>(fetch, t, 'PUT', '/v1/profile', { full_name: str(f, 'full_name'), phone: str(f, 'phone') });
		if (!r.ok) return fail(r.status === 0 ? 503 : 400, { section: 'profile', message: r.message });
		return { section: 'profile', ok: true };
	},

	changePassword: async ({ request, cookies, locals, fetch }) => {
		const t = token(cookies, locals);
		const f = await request.formData();
		const next = String(f.get('new_password') ?? '');
		if (next !== String(f.get('confirm_password') ?? '')) return fail(400, { section: 'password', code: 'MISMATCH' });
		const r = await call(fetch, t, 'POST', '/v1/profile/password', { current_password: String(f.get('current_password') ?? ''), new_password: next });
		if (!r.ok) return fail(r.status === 0 ? 503 : r.status === 429 ? 429 : 400, { section: 'password', code: r.body?.error?.code ?? 'ERR', message: r.message });
		return { section: 'password', ok: true };
	},

	// Mulai pendaftaran 2FA: kunci rahasia dan kode QR dibuat di sisi server; hanya tampil sekali di halaman ini.
	mfaStart: async ({ cookies, locals, fetch }) => {
		const t = token(cookies, locals);
		const r = await call<{ secret: string; qr_code_url: string; recovery_codes: string[] }>(fetch, t, 'POST', '/v1/auth/mfa/enroll', {});
		if (!r.ok) return fail(r.status === 0 ? 503 : 400, { section: 'mfa', message: r.message });
		const qr = await QRCode.toDataURL(r.value.qr_code_url, { margin: 1, width: 240, errorCorrectionLevel: 'M' });
		return { section: 'mfa', enroll: { secret: r.value.secret, qr, codes: r.value.recovery_codes } };
	},

	mfaConfirm: async ({ request, cookies, locals, fetch }) => {
		const t = token(cookies, locals);
		const f = await request.formData();
		const r = await call(fetch, t, 'POST', '/v1/auth/mfa/confirm', { code: str(f, 'code').replace(/\s+/g, '') });
		if (!r.ok) return fail(r.status === 0 ? 503 : 400, { section: 'mfa', code: r.body?.error?.code ?? 'ERR', message: r.message, keepEnroll: true });
		return { section: 'mfa', enabled: true };
	},

	mfaDisable: async ({ request, cookies, locals, fetch }) => {
		const t = token(cookies, locals);
		const f = await request.formData();
		const r = await call(fetch, t, 'POST', '/v1/auth/mfa/disable', { password: String(f.get('password') ?? ''), code: str(f, 'code').replace(/\s+/g, '') });
		if (!r.ok) return fail(r.status === 0 ? 503 : r.status, { section: 'mfa', code: r.body?.error?.code ?? 'ERR', message: r.message, proof: 'disable' });
		return { section: 'mfa', disabled: true };
	},

	mfaRegen: async ({ request, cookies, locals, fetch }) => {
		const t = token(cookies, locals);
		const f = await request.formData();
		const r = await call<{ recovery_codes: string[] }>(fetch, t, 'POST', '/v1/auth/mfa/regenerate-recovery-codes', {
			password: String(f.get('password') ?? ''),
			code: str(f, 'code').replace(/\s+/g, '')
		});
		if (!r.ok) return fail(r.status === 0 ? 503 : r.status, { section: 'mfa', code: r.body?.error?.code ?? 'ERR', message: r.message, proof: 'regen' });
		return { section: 'mfa', regen: r.value.recovery_codes };
	}
};
