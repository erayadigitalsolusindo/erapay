import { fail, redirect } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../../lib/server/auth';
import { SCOPES, createKey, listKeys, revokeKey, rotateKey } from '../../../lib/server/apikeys';
import type { Actions, PageServerLoad } from './$types';

const allowed = (locals: App.Locals) => !!locals.user?.merchantId && locals.user.permissions.includes('api_key:manage');

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	if (!allowed(locals)) redirect(303, '/dashboard');
	const token = cookies.get(SESSION_COOKIE);
	if (!token) redirect(303, '/login');
	// Scope yang ditawarkan dibatasi pada izin yang dimiliki pembuat (Go memeriksa ulang).
	const scopes = SCOPES.filter((s) => locals.user!.permissions.includes(s.id));
	return { keys: await listKeys(fetch, token), scopes };
};

export const actions: Actions = {
	create: async ({ request, cookies, fetch, locals }) => {
		const token = cookies.get(SESSION_COOKIE);
		if (!token || !allowed(locals)) redirect(303, '/login');
		const form = await request.formData();
		const name = String(form.get('name') ?? '').trim();
		const scopes = form.getAll('scopes').map(String);
		if (!name || name.length > 80) return fail(400, { message: 'Nama wajib diisi (maks. 80 karakter).', name });
		if (scopes.length === 0) return fail(400, { message: 'Pilih minimal satu izin.', name });
		const res = await createKey(fetch, token, name, scopes);
		if (!res.ok) return fail(400, { message: res.message, name });
		// Secret hanya ada di respons ini; tidak disimpan di server web.
		return { created: res.value };
	},
	revoke: async ({ request, cookies, fetch, locals }) => {
		const token = cookies.get(SESSION_COOKIE);
		if (!token || !allowed(locals)) redirect(303, '/login');
		const id = String((await request.formData()).get('id') ?? '');
		const res = await revokeKey(fetch, token, id);
		return res.ok ? { revoked: id } : fail(400, { message: res.message });
	},
	rotate: async ({ request, cookies, fetch, locals }) => {
		const token = cookies.get(SESSION_COOKIE);
		if (!token || !allowed(locals)) redirect(303, '/login');
		const f = await request.formData();
		const id = String(f.get('id') ?? '');
		const grace = Math.min(86400, Math.max(0, Number(f.get('grace') ?? 0) || 0));
		const res = await rotateKey(fetch, token, id, grace);
		return res.ok ? { created: res.value } : fail(400, { message: res.message });
	}
};
