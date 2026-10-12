<script lang="ts">
	import { getI18n } from '../i18n/i18n.svelte';
	// Status selalu berlabel teks, bukan warna saja (PRD §4.11).
	type Status = 'created' | 'pending' | 'succeeded' | 'failed' | 'expired' | 'cancelled' | 'unknown';
	let { status }: { status: Status } = $props();
	const { t } = getI18n();
	const warna: Record<Status, string> = {
		created: 'bg-ink/10 text-ink',
		pending: 'bg-amber-100 text-amber-800',
		unknown: 'bg-amber-100 text-amber-800',
		succeeded: 'bg-emerald-100 text-emerald-800',
		failed: 'bg-rose-100 text-rose-800',
		expired: 'bg-ink/10 text-ink',
		cancelled: 'bg-rose-100 text-rose-800'
	};
</script>

<span class="pill {warna[status]}">{t(`status.${status}`)}</span>
