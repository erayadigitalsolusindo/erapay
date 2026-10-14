import { isLang } from '../../../lib/i18n/messages';
import type { LayoutServerLoad } from './$types';

export const load: LayoutServerLoad = async ({ cookies }) => {
	const theme = cookies.get('erapay_theme');
	const lang = cookies.get('erapay_lang');
	return {
		theme: (theme === 'dark' || theme === 'light' ? theme : 'light') as 'light' | 'dark',
		lang: isLang(lang) ? lang : 'id'
	};
};
