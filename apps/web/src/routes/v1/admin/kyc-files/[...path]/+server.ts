// Proksi baca-saja berkas KYC untuk reviewer platform (KTP, dokumen legal, bukti bisnis, logo).
// Izin merchant:review dan kepemilikan diperiksa di Go; di sini hanya meneruskan sesi.
import { error } from '@sveltejs/kit';
import { SESSION_COOKIE } from '../../../../../lib/server/auth';
import type { RequestHandler } from './$types';

const BASE = process.env.ERAPAY_API_URL ?? 'http://localhost:8080';
const ALLOWED = /^(documents|evidences|logo)\/[0-9a-f-]+$/i;

export const GET: RequestHandler = async ({ params, cookies, fetch }) => {
	if (!ALLOWED.test(params.path)) error(404, 'Tidak ditemukan');
	const token = cookies.get(SESSION_COOKIE);
	if (!token) error(401, 'Belum masuk');
	const res = await fetch(`${BASE}/v1/admin/kyc-files/${params.path}`, {
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
