import { fail, redirect } from '@sveltejs/kit';
import { guardPlatform, platformApi, platformImpersonate } from '../../../../lib/server/platform';
import { setSession } from '../../../../lib/server/auth';
import type { Actions, PageServerLoad } from './$types';

type Merchant = { id: string; display_name: string; legal_name: string; onboarding_status: string; created_at: string };

export const load: PageServerLoad = async ({ locals, cookies, fetch, url }) => {
	const token = guardPlatform(locals, cookies);
	const q = url.searchParams.get('q')?.trim() ?? '';
	const r = await platformApi<{ data: Merchant[] }>(fetch, token, 'GET', `/merchants?q=${encodeURIComponent(q)}&limit=50`);
	return { q, merchants: r.ok ? r.value.data ?? [] : [], error: r.ok ? null : r.message };
};

export const actions: Actions = {
	// DEWA masuk sebagai tenant. Alasan wajib; Go mencatat impersonation.start dan semua request berikutnya.
	impersonate: async ({ request, cookies, locals, fetch, getClientAddress, url }) => {
		const token = guardPlatform(locals, cookies);
		const f = await request.formData();
		const merchantId = String(f.get('merchant_id') ?? '');
		const reason = String(f.get('reason') ?? '').trim();
		if (reason.length < 10) return fail(400, { merchantId, reason, message: 'Alasan wajib diisi (minimal 10 karakter).' });
		const r = await platformImpersonate(fetch, getClientAddress(), token, merchantId, reason);
		if (!r.ok) return fail(r.status === 0 ? 503 : r.status, { merchantId, reason, message: r.message });
		setSession(cookies, r.token, r.maxAge, url.protocol === 'https:');
		redirect(303, '/dashboard');
	}
};
