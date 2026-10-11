<script lang="ts">
	// Status selalu berlabel teks, bukan warna saja (PRD §4.11).
	type Status = 'created' | 'pending' | 'succeeded' | 'failed' | 'expired' | 'cancelled' | 'unknown';
	let { status }: { status: Status } = $props();
	const label: Record<Status, string> = {
		created: 'Dibuat',
		pending: 'Menunggu pembayaran',
		succeeded: 'Pembayaran berhasil',
		failed: 'Gagal',
		expired: 'Kedaluwarsa',
		cancelled: 'Dibatalkan',
		unknown: 'Belum pasti (sedang dicek)'
	};
</script>

<span class="badge {status}">{label[status]}</span>

<style>
	.badge { padding: 0.15rem 0.5rem; border-radius: 999px; font-size: 0.8rem; background: #e5e7eb; }
	.succeeded { background: #d1fae5; }
	.failed, .expired, .cancelled { background: #fee2e2; }
	.pending, .unknown { background: #fef3c7; }
</style>
