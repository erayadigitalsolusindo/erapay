import type { Handle } from '@sveltejs/kit/hooks';
import { SESSION_COOKIE, fetchMe } from './lib/server/auth';
import { PLATFORM_COOKIE, fetchPlatformMe } from './lib/server/platform';

/** Berjalan di setiap request: validasi sesi ke Go API (sesi bisa dicabut kapan saja). */
export const handle: Handle = async ({ event, resolve }) => {
	event.locals.requestId = crypto.randomUUID();
	event.locals.user = null;
	event.locals.platformUser = null;
	const token = event.cookies.get(SESSION_COOKIE);
	const ptoken = event.cookies.get(PLATFORM_COOKIE);

	// Dua pengecekan sesi independen → jalankan paralel, bukan berurutan.
	const [me, pme] = await Promise.all([
		token ? fetchMe(event.fetch, token) : null,
		ptoken ? fetchPlatformMe(event.fetch, ptoken) : null
	]);

	// 'unavailable' = API sedang bermasalah: pengguna dianggap belum login untuk request ini,
	// tetapi cookie dipertahankan agar sesi pulih sendiri saat API kembali.
	if (me === 'unavailable') {
		event.locals.user = null;
	} else {
		event.locals.user = me;
		if (token && !me) event.cookies.delete(SESSION_COOKIE, { path: '/' });
	}
	if (pme === 'unavailable') {
		event.locals.platformUser = null;
	} else {
		event.locals.platformUser = pme;
		if (ptoken && !pme) event.cookies.delete(PLATFORM_COOKIE, { path: '/platform' });
	}
	return resolve(event);
};
