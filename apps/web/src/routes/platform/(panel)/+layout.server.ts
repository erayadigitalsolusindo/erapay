import { guardPlatform } from '../../../lib/server/platform';
import { isLang } from '../../../lib/i18n/messages';
import type { LayoutServerLoad } from './$types';

// Penjaga seluruh halaman panel platform: hanya sesi DEWA yang lewat.
export const load: LayoutServerLoad = async ({ locals, cookies }) => {
	guardPlatform(locals, cookies);
	const theme = cookies.get('erapay_theme');
	const lang = cookies.get('erapay_lang');
	return {
		user: locals.platformUser!,
		theme: (theme === 'dark' || theme === 'light' ? theme : 'light') as 'light' | 'dark',
		lang: isLang(lang) ? lang : 'id'
	};
};
