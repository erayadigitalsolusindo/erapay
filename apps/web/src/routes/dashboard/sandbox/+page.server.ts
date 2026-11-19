import { fail, redirect } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../../lib/server/auth';
import { call, read } from '../../../lib/server/gw';
import type { Actions, PageServerLoad } from './$types';

export type SandboxStatus = { available: boolean; enabled: boolean; active: boolean; merchant_id: string; simulator: boolean };

function session(locals: App.Locals, cookies: import('@sveltejs/kit').Cookies) {
	const token = cookies.get(SESSION_COOKIE);
	if (!locals.user || !token) redirect(303, '/login');
	if (!locals.user.merchantId || !locals.user.permissions.includes('merchant:read')) redirect(303, '/dashboard');
	return token;
}

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = session(locals, cookies);
	return {
		status: await read<SandboxStatus>(fetch, token, '/v1/sandbox'),
		canEnable: locals.user!.permissions.includes('merchant:write')
	};
};

// Semua aturan (siapa boleh, merchant harus disetujui, tenant dari sesi) ada di Go; di sini hanya meneruskan.
export const actions: Actions = {
	enable: async ({ locals, cookies, fetch }) => {
		const token = session(locals, cookies);
		const r = await call(fetch, token, 'POST', '/v1/sandbox', {});
		if (!r.ok) return fail(r.status === 409 ? 409 : 400, { error: r.status === 409 ? 'notApproved' : 'generic' });
		return { done: true };
	},
	switch: async ({ locals, cookies, fetch, request }) => {
		const token = session(locals, cookies);
		const mode = String((await request.formData()).get('mode') ?? '');
		if (mode !== 'sandbox' && mode !== 'live') return fail(400, { error: 'generic' });
		const r = await call(fetch, token, 'POST', '/v1/sandbox/switch', { mode });
		if (!r.ok) return fail(400, { error: 'generic' });
		redirect(303, '/dashboard');
	}
};
