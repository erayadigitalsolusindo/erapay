// Uang = integer satuan terkecil (Rupiah penuh). Tidak pernah float (PRD §0 #1).
const idr = new Intl.NumberFormat('id-ID', { style: 'currency', currency: 'IDR', maximumFractionDigits: 0 });

export function formatIDR(amount: number): string {
	if (!Number.isSafeInteger(amount)) throw new Error('amount harus integer');
	return idr.format(amount);
}
