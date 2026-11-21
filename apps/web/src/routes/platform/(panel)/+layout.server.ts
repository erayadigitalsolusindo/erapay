import { guardPlatform, PLATFORM_COOKIE } from '../../../lib/server/platform';
import { isLang } from '../../../lib/i18n/messages';
import { read } from '../../../lib/server/gw';
import type { LayoutServerLoad } from './$types';

// Penjaga seluruh halaman panel platform: hanya sesi DEWA yang lewat.
export const load: LayoutServerLoad = async ({ locals, cookies, fetch }) => {
	guardPlatform(locals, cookies);
	const theme = cookies.get('erapay_theme');
	const lang = cookies.get('erapay_lang');
	// Menu finance/ops yang diizinkan hak akses DEWA (menu_role_access), sama seperti sumber menu dashboard.
	const menus = (await read<{ data?: string[] }>(fetch, cookies.get(PLATFORM_COOKIE)!, '/v1/menus'))?.data ?? [];
	return {
		menus,
		user: locals.platformUser!,
		theme: (theme === 'dark' || theme === 'light' ? theme : 'light') as 'light' | 'dark',
		lang: isLang(lang) ? lang : 'id'
	};
};
