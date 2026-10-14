<script lang="ts">
	let { data } = $props();
</script>

<svelte:head><title>Ringkasan Panel Platform · EraPay</title></svelte:head>

<div class="space-y-6">
	<div class="card p-6">
		<h1 class="text-xl font-bold">Selamat datang, {data.user.email.split('@')[0]}</h1>
		<p class="muted mt-1 text-sm">
			Panel platform khusus DEWA. Akses terbatas: hanya <b>Platform Admin</b> (rbac:manage) yang dapat masuk.
		</p>
	</div>

	<div class="grid gap-5 md:grid-cols-2">
		<a href="/platform/access" class="card group p-5 transition hover:-translate-y-0.5 hover:shadow-lg">
			<div class="flex items-center gap-3">
				<span class="grid size-10 place-items-center rounded-xl bg-merah text-white">
					<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
						<rect x="4" y="11" width="16" height="9" rx="2" /><path d="M8 11V8a4 4 0 0 1 8 0v3" />
					</svg>
				</span>
				<div>
					<div class="text-sm font-bold group-hover:text-merah">Hak akses (RBAC)</div>
					<div class="muted text-xs">Kelola role, izin, menu, dan user.</div>
				</div>
			</div>
		</a>

		<div class="card p-5">
			<div class="flex items-center gap-3">
				<span class="grid size-10 place-items-center rounded-xl bg-emas text-black">
					<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true">
						<path d="M5 7h14l-2 12H7L5 7Z" /><path d="M8 7V6a4 4 0 0 1 8 0v1" />
					</svg>
				</span>
				<div>
					<div class="text-sm font-bold">Mode kerajaan — dilarang dana nyata tanpa go/no-go</div>
					<div class="muted text-xs">DEWA kini di panel terpisah; dashboard tetap untuk super_admin/finance/ops.</div>
				</div>
			</div>
		</div>

		<div class="card p-5 md:col-span-2">
			<div class="muted text-xs leading-relaxed">
				<b>Catatan:</b> role, permission, dan menu dikelola via
				<code class="rounded bg-ink/5 px-1 py-0.5 font-mono text-[11px] text-merah-gelap">/v1/platform/admin/*</code>.
				Sesi panel memakai cookie <code class="rounded bg-ink/5 px-1 py-0.5 font-mono text-[11px] text-merah-gelap">erapay_platform_session</code>
				(Path <code>/platform</code>), terpisah dari sesi dashboard. Bootstrap hanya sekali via
				<code class="rounded bg-ink/5 px-1 py-0.5 font-mono text-[11px] text-merah-gelap">/platform/setup</code> +
				env token. Setiap perubahan finansial tetap wajib uji idempotency & concurrency di API Go.
			</div>
		</div>
	</div>
</div>
