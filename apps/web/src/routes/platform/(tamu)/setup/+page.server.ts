import { fail, redirect } from '@sveltejs/kit';
import { platformSetup, platformSetupStatus } from '../../../../lib/server/platform';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, fetch }) => {
	if (locals.platformUser) redirect(303, '/platform');
	const status = await platformSetupStatus(fetch);
	return { status };
};

export const actions: Actions = {
	default: async ({ request, fetch, getClientAddress }) => {
		const form = await request.formData();
		const token = String(form.get('token') ?? '').trim();
		const email = String(form.get('email') ?? '').trim();
		const password = String(form.get('password') ?? '');
		if (!token || !email || !password) return fail(400, { message: 'Token, email, dan kata sandi wajib diisi.' });
		const r = await platformSetup(fetch, getClientAddress(), token, email, password);
		if (!r.ok) {
			if (r.status === 409) return { status: 'done' as const, error: null };
			return fail(r.status === 401 ? 401 : 400, { message: r.message });
		}
		return { status: 'created' as const, error: null };
	}
};
