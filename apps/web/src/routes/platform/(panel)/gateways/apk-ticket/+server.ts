import { json } from '@sveltejs/kit';
import { API_HOST } from '../../../../../lib/utils/apiHost';
import { guardPlatform, platformApi } from '../../../../../lib/server/platform';
import type { RequestHandler } from './$types';

// Tiket sekali pakai agar browser bisa mengunggah APK langsung ke API Go (cookie sesi httpOnly tidak ikut ke domain API).
export const POST: RequestHandler = async ({ cookies, locals, fetch }) => {
	const token = guardPlatform(locals, cookies);
	const r = await platformApi<{ ticket: string }>(fetch, token, 'POST', '/listener-apk/ticket', {});
	if (!r.ok) return json({ message: r.message }, { status: r.status || 503 });
	const base = (process.env.ERAPAY_PUBLIC_API_URL || API_HOST).replace(/\/$/, '');
	return json({ url: `${base}/v1/platform/listener-apk/upload?ticket=${encodeURIComponent(r.value.ticket)}` });
};
