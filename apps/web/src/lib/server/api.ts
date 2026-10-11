// Klien untuk memanggil Go API. HANYA dipakai di sisi server (folder lib/server
// tidak bisa di-import dari kode browser: SvelteKit akan menolak saat build).
// Semua validasi & logika keuangan ada di Go, bukan di sini (PRD §0 #13).
// Dibaca dari environment saat server berjalan (lihat .env.example).

const BASE = process.env.ERAPAY_API_URL ?? 'http://localhost:8080';

export async function apiGet<T>(path: string, fetchFn: typeof fetch = fetch): Promise<T> {
	const res = await fetchFn(`${BASE}${path}`, { headers: { accept: 'application/json' } });
	if (!res.ok) throw new Error(`API ${path} → ${res.status}`);
	return (await res.json()) as T;
}
