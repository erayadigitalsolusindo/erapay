import type { RequestHandler } from './$types';

const BASE = process.env.ERAPAY_API_URL ?? 'http://localhost:8080';

// Unduh APK listener terbaru tanpa login (dipakai tombol di login ARUS Mobile); berkas dan datanya ada di Go.
export const GET: RequestHandler = async ({ fetch }) => {
	let res: Response;
	try {
		res = await fetch(`${BASE}/v1/public/listener/apk`);
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
