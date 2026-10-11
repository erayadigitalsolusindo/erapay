import type { Handle } from '@sveltejs/kit/hooks';
import { SESSION_COOKIE, fetchMe } from './lib/server/auth';

/** Berjalan di setiap request: validasi sesi ke Go API (sesi bisa dicabut kapan saja). */
export const handle: Handle = async ({ event, resolve }) => {
	event.locals.requestId = crypto.randomUUID();
	event.locals.user = null;
	const token = event.cookies.get(SESSION_COOKIE);
	if (token) {
		event.locals.user = await fetchMe(event.fetch, token);
		if (!event.locals.user) event.cookies.delete(SESSION_COOKIE, { path: '/' });
	}
	return resolve(event);
};
