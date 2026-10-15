// Kontrak yang diimplementasikan halaman (wizard / Status Akun) memakai lib/server/kyc.ts.
import type { Region } from '../../server/kyc';

export type SectionApi = {
	/** Simpan bagian: company | representative | brand. Kembalikan field errors bila gagal. */
	save: (
		section: string,
		payload: Record<string, unknown>
	) => Promise<{ ok: boolean; fields?: Record<string, string>; message?: string }>;
	/** Kirim seluruh isian untuk verifikasi (DRAFT/NEEDS_REVISION -> SUBMITTED). */
	submit: () => Promise<{ ok: boolean; fields?: Record<string, string>; message?: string }>;
	/** Unggah dokumen wajib (docType: ktp|npwp_badan|akta|sk_kemenkumham|nib|izin_usaha). */
	uploadDoc: (
		docType: string,
		file: File,
		extra?: { number?: string; kbli?: string; confirm?: boolean }
	) => Promise<{ ok: boolean; fields?: Record<string, string>; message?: string }>;
	/** Ganti logo merek (gambar ≤ 5 MB). */
	uploadLogo: (file: File) => Promise<{ ok: boolean; message?: string }>;
	/** Tambah bukti bisnis (gambar ≤ 5 MB). */
	addEvidence: (file: File) => Promise<{ ok: boolean; message?: string }>;
	/** Hapus bukti bisnis. */
	deleteEvidence: (id: string) => Promise<{ ok: boolean; message?: string }>;
	/** Autocomplete wilayah. */
	regions: (q: string) => Promise<Region[]>;
};
