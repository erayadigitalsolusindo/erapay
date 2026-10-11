import { redirect } from '@sveltejs/kit';
import { SESSION_COOKIE, logout } from '../../lib/server/auth';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = () => redirect(303, '/login');

export const actions: Actions = {
	default: async ({ cookies, fetch }) => {
		const token = cookies.get(SESSION_COOKIE);
		if (token) await logout(fetch, token); // cabut sesi di server, bukan sekadar hapus cookie
		cookies.delete(SESSION_COOKIE, { path: '/' });
		redirect(303, '/login');
	}
};
