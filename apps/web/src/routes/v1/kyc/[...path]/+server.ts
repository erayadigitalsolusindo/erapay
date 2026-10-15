// Proksi baca-saja untuk berkas KYC (KTP, logo, bukti bisnis) agar <img src="/v1/kyc/…"> bisa
// dimuat browser. Sesi diteruskan ke Go; kepemilikan file diperiksa di sana (bukan di sini).
import { error } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../../../lib/server/auth';
import type { RequestHandler } from './$types';

const BASE = process.env.ERAPAY_API_URL ?? 'http://localhost:8080';
const ALLOWED = /^(documents\/[0-9a-f-]+\/file|evidences\/[0-9a-f-]+\/file|brand\/logo\/file)$/i;

export const GET: RequestHandler = async ({ params, cookies, fetch, url }) => {
	if (!ALLOWED.test(params.path)) error(404, 'Tidak ditemukan');
	const token = cookies.get(SESSION_COOKIE);
	if (!token) error(401, 'Belum masuk');
	const res = await fetch(`${BASE}/v1/kyc/${params.path}${url.search}`, {
		headers: { cookie: `${SESSION_COOKIE}=${token}` }
	});
	if (!res.ok) error(res.status === 403 ? 403 : 404, 'Berkas tidak tersedia');
	const headers = new Headers();
	for (const h of ['content-type', 'content-disposition', 'content-length']) {
		const v = res.headers.get(h);
		if (v) headers.set(h, v);
	}
	headers.set('x-content-type-options', 'nosniff');
	headers.set('cache-control', 'private, max-age=300');
	return new Response(res.body, { status: 200, headers });
};
