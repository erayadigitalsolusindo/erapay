import { redirect } from '@sveltejs/kit';
import { PLATFORM_COOKIE, platformLogout } from '../../../lib/server/platform';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = () => redirect(303, '/platform');

export const actions: Actions = {
	default: async ({ cookies, fetch }) => {
		const token = cookies.get(PLATFORM_COOKIE);
		if (token) await platformLogout(fetch, token); // cabut sesi di server, bukan sekadar hapus cookie
		cookies.delete(PLATFORM_COOKIE, { path: '/platform' });
		redirect(303, '/platform/login');
	}
};
