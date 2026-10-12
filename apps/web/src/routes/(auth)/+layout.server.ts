import { isLang } from '../../lib/i18n/messages';
import type { LayoutServerLoad } from './$types';

// Tema & bahasa halaman masuk/daftar dibaca dari cookie yang sama dengan dasbor.
export const load: LayoutServerLoad = ({ cookies }) => {
	const theme = cookies.get('erapay_theme');
	const lang = cookies.get('erapay_lang');
	return {
		theme: (theme === 'dark' || theme === 'light' ? theme : 'system') as 'light' | 'dark' | 'system',
		lang: isLang(lang) ? lang : 'id'
	};
};
