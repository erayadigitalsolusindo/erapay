import type { Handle } from '@sveltejs/kit/hooks';

/**
 * Berjalan di SETIAP request (middleware).
 * TODO (langkah 3 PRD): validasi cookie sesi ke Go API lalu isi locals.user.
 * Selama auth backend belum ada, user selalu null → /dashboard diarahkan ke /login.
 */
export const handle: Handle = async ({ event, resolve }) => {
	event.locals.requestId = crypto.randomUUID();
	event.locals.user = null;
	return resolve(event);
};
