import { read } from '../../../lib/server/gw';
import { redirect } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../../lib/server/auth';
import type { PageServerLoad } from './$types';

// Halaman merchant sendiri (scope merchant): hanya memakai /v1/merchant/*, bukan /v1/admin.
export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const u = locals.user;
	const token = cookies.get(SESSION_COOKIE);
	if (!u || !token) redirect(303, '/login');
	if (!u.merchantId || !u.permissions.includes('merchant:read')) redirect(303, '/dashboard');
	const [profile, outlets] = await Promise.all([
		read<{ id: string; legal_name: string; display_name: string; onboarding_status: string; review_note: string | null }>(
			fetch,
			token,
			'/v1/merchant/profile'
		),
		u.permissions.includes('outlet:read') ? read<{ data: { id: string; name: string }[] }>(fetch, token, '/v1/merchant/outlets') : Promise.resolve({ data: [] })
	]);
	return { profile, outlets: outlets?.data ?? null };
};
