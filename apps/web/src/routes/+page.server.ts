import { isLang } from '../lib/i18n/messages';
import type { PageServerLoad } from './$types';

// Bahasa landing page dari cookie yang sama dengan dasbor, supaya SSR langsung dalam bahasa pilihan.
export const load: PageServerLoad = ({ cookies }) => {
	const lang = cookies.get('erapay_lang');
	return { lang: isLang(lang) ? lang : 'id' };
};
