import { getContext, setContext } from 'svelte';
import { messages, type Key, type Lang } from './messages';

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
			return lang === 'en' ? 'en-US' : 'id-ID';
		},
		t(key: Key, params?: Record<string, string | number>): string {
			let s = messages[lang][key];
			for (const [k, v] of Object.entries(params ?? {})) s = s.replaceAll(`{${k}}`, String(v));
			return s;
		}
	};
}

export type I18n = ReturnType<typeof createI18n>;
export const setI18n = (i: I18n) => setContext('i18n', i);
export const getI18n = () => getContext<I18n>('i18n');
