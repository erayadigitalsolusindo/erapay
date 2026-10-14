import type { Handle } from '@sveltejs/kit/hooks';
import { SESSION_COOKIE, fetchMe } from './lib/server/auth';
import { PLATFORM_COOKIE, fetchPlatformMe } from './lib/server/platform';

/** Berjalan di setiap request: validasi sesi ke Go API (sesi bisa dicabut kapan saja). */
export const handle: Handle = async ({ event, resolve }) => {
	event.locals.requestId = crypto.randomUUID();
	event.locals.user = null;
	event.locals.platformUser = null;
	const token = event.cookies.get(SESSION_COOKIE);
	if (token) {
		event.locals.user = await fetchMe(event.fetch, token);
		if (!event.locals.user) event.cookies.delete(SESSION_COOKIE, { path: '/' });
	}
	const ptoken = event.cookies.get(PLATFORM_COOKIE);
	if (ptoken) {
		event.locals.platformUser = await fetchPlatformMe(event.fetch, ptoken);
		if (!event.locals.platformUser) event.cookies.delete(PLATFORM_COOKIE, { path: '/platform' });
	}
	return resolve(event);
};
