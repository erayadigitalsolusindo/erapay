import { fail } from '@sveltejs/kit';
import { requestEmail } from '../../../lib/server/auth';
import { guard } from '../../../lib/server/gw';
import {
	loadProfile,
	saveSection,
	submitKyc,
	uploadDocument,
	setBrandLogo,
	addEvidence,
	deleteEvidence,
	searchRegions,
	type Profile
} from '../../../lib/server/kyc';
import type { Actions, PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ locals, cookies, fetch }) => {
	const token = guard(locals, cookies, 'merchant:write', 'merchant');
	return {
		profile: await loadProfile(fetch, token),
		email: locals.user!.email,
		emailVerified: locals.user!.emailVerified
	};
};

// Terjemahkan hasil panggilan Go menjadi respons action: {ok:true, profile} atau fail().
// Semua aturan (validasi, izin, batas) tetap di Go; di sini hanya meneruskan sesi.
function done(r: { ok: boolean; message?: string; fields?: Record<string, string>; value?: Profile }, status = 400) {
	if (r.ok) return { ok: true, profile: r.value ?? null };
	return fail(status, { message: r.message ?? 'Gagal.', fields: r.fields });
}

export const actions: Actions = {
	// Alamat diambil dari sesi, bukan dari form: tombol ini hanya untuk akun yang sedang login.
	resend: async ({ locals, cookies, fetch, getClientAddress }) => {
		guard(locals, cookies, 'merchant:write', 'merchant');
		const r = await requestEmail(fetch, getClientAddress(), 'resend-verification', locals.user!.email);
		return r.ok ? { resent: true } : fail(r.code === 'rate_limited' ? 429 : 503, { message: r.code === 'rate_limited' ? 'Terlalu banyak permintaan. Coba lagi nanti.' : 'Layanan sedang tidak tersedia.' });
	},
	// Simpan satu bagian wizard KYC (company|representative|brand). Body JSON dari komponen.
	kycSave: async ({ locals, cookies, fetch, request }) => {
		const token = guard(locals, cookies, 'merchant:write', 'merchant');
		const body = await readJson(request);
		if (!body || typeof body.section !== 'string') return fail(400, { message: 'Bagian tidak valid.' });
		// Go mengharapkan {company|representative|brand: {...}, confirm_sensitive}, bukan payload datar.
			const r = await saveSection(fetch, token, body.section, {
				[body.section]: body.payload ?? {},
				...(body.confirm_sensitive ? { confirm_sensitive: true } : {})
			});
		return done(r);
	},
	// Kirim seluruh isian untuk verifikasi (DRAFT/REJECTED/NEEDS_REVISION → SUBMITTED).
	kycSubmit: async ({ locals, cookies, fetch }) => {
		const token = guard(locals, cookies, 'merchant:write', 'merchant');
		const r = await submitKyc(fetch, token);
		return done(r);
	},
	// Unggah dokumen wajib (multipart: file, number, kbli, confirm).
	kycUpload: async ({ locals, cookies, fetch, request }) => {
		const token = guard(locals, cookies, 'merchant:write', 'merchant');
		const f = await request.formData();
		const file = f.get('file');
		if (!(file instanceof File)) return fail(400, { message: 'Berkas wajib diisi.' });
		const docType = String(f.get('doc_type') ?? '');
		const extra: { number?: string; kbli?: string; confirm?: boolean } = {};
		const number = String(f.get('number') ?? '');
		const kbli = String(f.get('kbli') ?? '');
		if (number) extra.number = number;
		if (kbli) extra.kbli = kbli;
		if (f.get('confirm') === 'true') extra.confirm = true;
		const r = await uploadDocument(fetch, token, docType, fileToFileLike(file), extra);
		const profile = await loadProfile(fetch, token);
		return r.ok
			? { ok: true, profile }
			: fail(r.status === 409 ? 409 : 400, { message: r.message, fields: r.fields });
	},
	// Ganti logo merek (multipart: file).
	kycLogo: async ({ locals, cookies, fetch, request }) => {
		const token = guard(locals, cookies, 'merchant:write', 'merchant');
		const f = await request.formData();
		const file = f.get('file');
		if (!(file instanceof File)) return fail(400, { message: 'Berkas wajib diisi.' });
		const r = await setBrandLogo(fetch, token, fileToFileLike(file));
		const profile = await loadProfile(fetch, token);
		return r.ok ? { ok: true, profile } : fail(400, { message: r.message });
	},
	// Tambah bukti bisnis (multipart: file).
	kycEvidence: async ({ locals, cookies, fetch, request }) => {
		const token = guard(locals, cookies, 'merchant:write', 'merchant');
		const f = await request.formData();
		const file = f.get('file');
		if (!(file instanceof File)) return fail(400, { message: 'Berkas wajib diisi.' });
		const r = await addEvidence(fetch, token, fileToFileLike(file));
		const profile = await loadProfile(fetch, token);
		return r.ok ? { ok: true, profile } : fail(400, { message: r.message });
	},
	// Hapus bukti bisnis.
	kycDeleteEvidence: async ({ locals, cookies, fetch, request }) => {
		const token = guard(locals, cookies, 'merchant:write', 'merchant');
		const body = await readJson(request);
		if (!body || typeof body.id !== 'string') return fail(400, { message: 'ID tidak valid.' });
		const r = await deleteEvidence(fetch, token, body.id);
		const profile = await loadProfile(fetch, token);
		return r.ok ? { ok: true, profile } : fail(400, { message: r.message });
	},
	// Autocomplete wilayah (kode pos / kelurahan / kota) untuk form alamat.
	kycRegions: async ({ locals, cookies, fetch, request }) => {
		const token = guard(locals, cookies, 'merchant:write', 'merchant');
		const body = await readJson(request);
		const q = typeof body?.q === 'string' ? body.q : '';
		const regions = await searchRegions(fetch, token, q);
		return { ok: true, regions };
	}
};

// SvelteKit menolak (415) body non-form pada action, jadi komponen mengirim JSON dalam field form "json".
async function readJson(request: Request): Promise<any> {
	try {
		return JSON.parse(String((await request.formData()).get('json') ?? 'null'));
	} catch {
		return null;
	}
}

// SvelteKit File dari formData dikonversi menjadi File (File sudah subclass Blob;
// fungsi upload memakai parameter bertipe File/Blob).
function fileToFileLike(file: File): File {
	return file;
}
