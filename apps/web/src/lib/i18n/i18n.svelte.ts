import { getContext, setContext } from 'svelte';
import { messages, LOCALES, isLang, type Key, type Lang } from './messages';

function fill(s: string, params?: Record<string, string | number>) {
	for (const [k, v] of Object.entries(params ?? {})) s = s.replaceAll(`{${k}}`, String(v));
	return s;
}

// Dibuat per-layout lewat context (bukan singleton modul) supaya state tidak bocor antar request SSR.
export function createI18n(initial: Lang) {
	let lang = $state<Lang>(initial);
	return {
		get lang() {
			return lang;
		},
		set lang(v: Lang) {
			lang = v;
		},
		get locale() {
			return LOCALES[lang];
		},
		t(key: Key, params?: Record<string, string | number>): string {
			return fill(messages[lang][key], params);
		},
		/** Untuk nilai dari server (status, enum) yang kuncinya dibentuk dinamis; jika tak ada terjemahan, pakai `fallback`. */
		tx(key: string, fallback: string, params?: Record<string, string | number>): string {
			const m = messages[lang] as Record<string, string>;
			return key in m ? fill(m[key], params) : fallback;
		}
	};
}

/** Terjemahan di luar komponen (mis. dialog global): bahasa dibaca dari <html lang>. */
export function tNow(key: Key, params?: Record<string, string | number>): string {
	const l = typeof document !== 'undefined' ? document.documentElement.lang : '';
	return fill(messages[isLang(l) ? l : 'id'][key], params);
}

export type I18n = ReturnType<typeof createI18n>;
export const setI18n = (i: I18n) => setContext('i18n', i);
export const getI18n = () => getContext<I18n>('i18n');
