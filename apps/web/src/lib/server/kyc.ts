// Klien KYC ke Go API (pola gw.ts): sesi diteruskan, semua aturan ada di Go.
// NIK/NPWP hanya kembali terbuka pada GET /v1/kyc/profile/edit (mode edit pemilik).
import { SESSION_COOKIE } from './auth';

const BASE = process.env.ERAPAY_API_URL ?? 'http://localhost:8080';

export type Result<T> =
	| { ok: true; value: T; status: number }
	| {
			ok: false;
			message: string;
			status: number;
			fields?: Record<string, string>;
			code?: string;
	  };

async function parseBody(res: Response) {
	const json = await res.json().catch(() => null);
	if (res.ok) return { ok: true as const, value: json, status: res.status };
	return {
		ok: false as const,
		message: json?.error?.message ?? 'Permintaan gagal.',
		status: res.status,
		fields: json?.error?.fields as Record<string, string> | undefined,
		code: json?.error?.code as string | undefined
	};
}

/** Panggilan JSON umum sebagai merchant yang sedang login. */
export async function call<T = unknown>(
	f: typeof fetch,
	token: string,
	method: 'GET' | 'POST' | 'PUT' | 'DELETE',
	path: string,
	body?: unknown
): Promise<Result<T>> {
	let res: Response;
	try {
		res = await f(`${BASE}${path}`, {
			method,
			headers: { 'content-type': 'application/json', cookie: `${SESSION_COOKIE}=${token}` },
			body: method === 'GET' ? undefined : JSON.stringify(body ?? {})
		});
	} catch {
		return { ok: false, message: 'Layanan tidak dapat dihubungi.', status: 0 };
	}
	const b = await parseBody(res);
	return b.ok ? (b as { ok: true; value: T; status: number }) : b;
}

/** Unggah multipart (dokumen/logo/bukti). Batas ukuran & MIME dicek ulang di Go. */
export async function upload<T = unknown>(
	f: typeof fetch,
	token: string,
	path: string,
	file: File,
	fields: Record<string, string> = {}
): Promise<Result<T>> {
	const fd = new FormData();
	fd.append('file', file);
	for (const [k, v] of Object.entries(fields)) fd.append(k, v);
	let res: Response;
	try {
		res = await f(`${BASE}${path}`, {
			method: 'POST',
			headers: { cookie: `${SESSION_COOKIE}=${token}` },
			body: fd
		});
	} catch {
		return { ok: false, message: 'Layanan tidak dapat dihubungi.', status: 0 };
	}
	const b = await parseBody(res);
	return b.ok ? (b as { ok: true; value: T; status: number }) : b;
}

// ---- tipe profil KYC (mengikuti JSON Go internal/kyc/model.go) ----

export type Company = {
	entity_type: string;
	company_name: string;
	phone: string;
	email: string;
	postal_code: string;
	kelurahan: string;
	kecamatan: string;
	kota: string;
	provinsi: string;
	address_detail: string;
	business_id: string;
	complete: boolean;
};

export type Representative = {
	name: string;
	position: string;
	phone: string;
	email: string;
	nationality: string;
	id_masked: string; // "••••1234" (server)
	id_number: string; // terisi hanya pada profil edit (reveal pemilik)
	ktp?: (KycFile & { id: string }) | null;
	ktp_back?: (KycFile & { id: string }) | null;
	id_confirmed: boolean;
	complete: boolean;
};

export type KycFile = { url: string; name: string; mime: string; size: number };
export type Link = { id: string; url: string };
export type Evidence = { id: string; url: string; name: string; mime: string; size: number };
export type Brand = {
	brand_id: string;
	name: string;
	category: string;
	description: string;
	monthly_amount: number;
	monthly_count: number;
	logo: KycFile | null;
	links: Link[];
	evidences: Evidence[];
	complete: boolean;
};
export type DocKind = 'ktp' | 'npwp_badan' | 'akta' | 'sk_kemenkumham' | 'nib' | 'izin_usaha';
export type Document = KycFile & { id: string };
export type Documents = {
	[P in DocKind]: Document | null;
};

export type Note = { id: string; section: string; field?: string; note: string; resolved: boolean };
export type SectionCheck = {
	section: string;
	label: string;
	complete: boolean;
	needs_revision: boolean;
	recheck?: boolean;
};

export type Profile = {
	status: string;
	recheck: boolean;
	submitted_at?: string;
	reviewed_at?: string;
	review_note?: string;
	editable: boolean;
	locked: boolean;
	company: Company;
	representative: Representative;
	brand: Brand;
	documents: Documents;
	checklist: SectionCheck[];
	notes: Note[];
	recheck_policy: 'open' | 'closed';
};

export type Region = {
	postal_code: string;
	kelurahan: string;
	kecamatan: string;
	kota: string;
	provinsi: string;
};

/** Profil KYC (ter-mask). null bila API gagal — halaman menampilkan pesan gagal muat. */
export async function loadProfile(f: typeof fetch, token: string): Promise<Profile | null> {
	const r = await call<Profile>(f, token, 'GET', '/v1/kyc/profile');
	return r.ok ? r.value : null;
}

/** Profil mode edit: NIK/NPWP terbuka; server menolak bila status terkunci. */
export async function loadProfileForEdit(f: typeof fetch, token: string): Promise<Profile | null> {
	const r = await call<Profile>(f, token, 'GET', '/v1/kyc/profile/edit');
	return r.ok ? r.value : null;
}

/** Simpan satu bagian (company|representative|brand). Sukses mengembalikan profil terbaru. */
export async function saveSection(
	f: typeof fetch,
	token: string,
	section: string,
	body: Record<string, unknown>
): Promise<Result<Profile>> {
	return call<Profile>(f, token, 'POST', `/v1/kyc/sections/${section}`, body);
}

/** Kirim seluruh isian untuk verifikasi (DRAFT/NEEDS_REVISION → SUBMITTED). */
export async function submitKyc(f: typeof fetch, token: string): Promise<Result<Profile>> {
	return call<Profile>(f, token, 'POST', '/v1/kyc/submit');
}

/** Unggah dokumen wajib; `number` = NPWP/NIB, `kbli` = 5 digit pasangan NIB. */
export function uploadDocument(
	f: typeof fetch,
	token: string,
	docType: string,
	file: File,
	fields: { number?: string; kbli?: string; confirm?: boolean } = {}
): Promise<Result<KycFile & { id?: string }>> {
	const extra: Record<string, string> = {};
	if (fields.number) extra.number = fields.number;
	if (fields.kbli) extra.kbli = fields.kbli;
	if (fields.confirm) extra.confirm = 'true';
	return upload<KycFile & { id: string }>(f, token, `/v1/kyc/documents/${docType}`, file, extra);
}

/** Ganti logo merek (gambar, maks 5 MB). */
export function setBrandLogo(f: typeof fetch, token: string, file: File) {
	return upload<never>(f, token, '/v1/kyc/brand/logo', file, {});
}

/** Tambah satu bukti bisnis (gambar, maks 5 MB). */
export function addEvidence(f: typeof fetch, token: string, file: File) {
	return upload<Evidence>(f, token, '/v1/kyc/evidences', file, {});
}

/** Hapus satu bukti bisnis milik merchant. */
export function deleteEvidence(f: typeof fetch, token: string, id: string) {
	return call(f, token, 'DELETE', `/v1/kyc/evidences/${id}`);
}

/** Autocomplete wilayah untuk alamat (kode pos / kelurahan / kota). */
export async function searchRegions(f: typeof fetch, token: string, q: string): Promise<Region[]> {
	const r = await call<{ data: Region[] }>(f, token, 'GET', `/v1/kyc/regions?q=${encodeURIComponent(q)}&limit=10`);
	return r.ok ? r.value.data : [];
}
