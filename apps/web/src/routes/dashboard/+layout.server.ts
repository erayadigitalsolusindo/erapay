import { redirect } from '@sveltejs/kit';
import { isLang } from '../../lib/i18n/messages';
import type { LayoutServerLoad } from './$types';

// Penjaga semua rute di bawah /dashboard. Tema & bahasa dibaca dari cookie agar SSR tanpa kedip.
export const load: LayoutServerLoad = ({ locals, cookies }) => {
	if (!locals.user) redirect(303, '/login');
	const theme = cookies.get('erapay_theme');
	const lang = cookies.get('erapay_lang');
	return {
		user: locals.user,
		theme: (theme === 'dark' || theme === 'light' ? theme : 'system') as 'light' | 'dark' | 'system',
		lang: isLang(lang) ? lang : 'id'
	};
};
