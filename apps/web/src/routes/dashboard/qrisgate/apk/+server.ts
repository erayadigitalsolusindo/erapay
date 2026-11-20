import { redirect } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../../../lib/server/auth';
import type { RequestHandler } from './$types';

const BASE = process.env.ERAPAY_API_URL ?? 'http://localhost:8080';

// Unduh APK listener terbaru: izin dan berkas ada di Go; di sini hanya meneruskan sesi dan mengalirkan hasilnya.
export const GET: RequestHandler = async ({ locals, cookies, fetch }) => {
	const token = cookies.get(SESSION_COOKIE);
	if (!locals.user || !token) redirect(303, '/login');
	if (!locals.user.merchantId || !locals.user.permissions.includes('merchant:read')) redirect(303, '/dashboard');
	let res: Response;
	try {
		res = await fetch(`${BASE}/v1/qrisgate/listener/download`, { headers: { cookie: `${SESSION_COOKIE}=${token}` } });
	} catch {
		return new Response('Layanan tidak dapat dihubungi.', { status: 502 });
	}
	if (!res.ok || !res.body) return new Response('APK belum tersedia.', { status: res.status === 404 ? 404 : 502 });
	const h = new Headers();
	for (const k of ['content-type', 'content-disposition', 'content-length', 'x-content-type-options', 'cache-control']) {
		const v = res.headers.get(k);
		if (v) h.set(k, v);
	}
	return new Response(res.body, { status: 200, headers: h });
};
