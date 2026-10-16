<script lang="ts">
	import { enhance } from '$app/forms';
	import { getI18n } from '../../../../../../lib/i18n/i18n.svelte';
	import Combobox from '../../../../../../lib/components/Combobox.svelte';

	let { data, form } = $props();
	const { tx } = getI18n();

	type Env = 'sandbox' | 'production';
	const ENVS: Env[] = ['sandbox', 'production'];

	// Hijau = siap menerima pembayaran, kuning = belum dikonfirmasi, merah = ditangguhkan.
	const STATUS: Record<string, { label: string; desc: string; pill: string; dot: string; bar: string }> = {
		active: {
			label: 'Aktif',
			desc: 'Merchant boleh membuat QRIS lewat akun ini. Hanya pilih ini setelah Merchant ID dipastikan benar.',
			pill: 'bg-emerald-500/15 text-emerald-700',
			dot: 'bg-emerald-500',
			bar: 'from-emerald-500 to-teal-600'
		},
		pending_confirmation: {
			label: 'Menunggu konfirmasi',
			desc: 'Data tersimpan tetapi akun belum dipakai. Pembuatan QRIS tetap ditolak sampai statusnya Aktif.',
			pill: 'bg-amber-500/15 text-amber-700',
			dot: 'bg-amber-500',
			bar: 'from-amber-400 to-orange-500'
		},
		suspended: {
			label: 'Ditangguhkan',
			desc: 'Dihentikan sementara. Pembuatan QRIS ditolak; data tetap tersimpan dan bisa diaktifkan lagi.',
			pill: 'bg-red-500/15 text-red-700',
			dot: 'bg-red-500',
			bar: 'from-red-500 to-rose-700'
		}
	};
	const STEP: Record<string, { title: string; desc: string }> = {
		credentials: { title: 'Kredensial gateway', desc: 'Client ID, secret, private key, dan base URL di server dapat meminta token ke gateway' },
		merchant: { title: 'Merchant ID', desc: 'Gateway menerima pembuatan QR uji Rp1.000 untuk Merchant ID ini' },
		cleanup: { title: 'Tutup QR uji', desc: 'QR uji langsung dikedaluwarsakan agar tidak bisa dibayar' }
	};
	const GW_TONE: Record<string, string> = { doku: 'from-sky-500 to-indigo-600' };
	const statusOptions = Object.entries(STATUS).map(([value, s]) => ({ value, label: s.label }));
	const envOptions = [
		{ value: 'sandbox', label: 'Sandbox (uji coba)' },
		{ value: 'production', label: 'Production (uang sungguhan)' }
	];

	type Acc = { id: string; gateway: string; environment: Env; gateway_ref?: string; activation_status: string; created_at: string };
	const serverEnv = $derived((data.overview?.server_environment ?? '') as Env | '');
	const gateways = $derived(data.overview?.gateways ?? ['doku']);
	const accounts = $derived((data.overview?.accounts ?? []) as Acc[]);
	const find = (g: string, e: Env) => accounts.find((a) => a.gateway === g && a.environment === e);
	const count = (s: string) => accounts.filter((a) => a.activation_status === s).length;
	const readyHere = $derived(accounts.some((a) => a.environment === serverEnv && a.activation_status === 'active' && a.gateway_ref));

	let testing = $state<string | null>(null);
	let saving = $state(false);
	let copied = $state(false);
	let fGateway = $state('doku');
	let fEnv = $state<string>('production');
	let fStatus = $state('active');
	let fRef = $state('');
	let formEl = $state<HTMLElement | null>(null);
	let seeded = false;
	$effect(() => {
		if (seeded || !data.overview) return;
		seeded = true;
		fGateway = data.overview.gateways[0] ?? 'doku';
		fEnv = data.overview.server_environment === 'sandbox' ? 'sandbox' : 'production';
	});

	function edit(g: string, e: Env) {
		const a = find(g, e);
		fGateway = g;
		fEnv = e;
		fStatus = a?.activation_status ?? 'active';
		fRef = a?.gateway_ref ?? '';
		formEl?.scrollIntoView({ behavior: 'smooth', block: 'center' });
		formEl?.querySelector<HTMLInputElement>('#gw-ref')?.focus({ preventScroll: true });
	}
	async function copyId() {
		try {
			await navigator.clipboard.writeText(data.id);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			/* clipboard tidak tersedia */
		}
	}
	const fmt = (s: string) => new Date(s).toLocaleDateString('id-ID', { day: '2-digit', month: 'short', year: 'numeric' });
</script>

<svelte:head><title>{tx('pf.gw.pageTitle', 'Akun gateway')}</title></svelte:head>

<div class="space-y-6">
	<!-- hero -->
	<section class="relative overflow-hidden rounded-3xl bg-linear-to-br from-merah-gelap via-merah-tua to-merah p-6 text-white shadow-lg sm:p-8">
		<div class="pointer-events-none absolute -right-16 -top-20 size-64 rounded-full bg-emas/25 blur-3xl"></div>
		<div class="pointer-events-none absolute -bottom-24 left-1/3 size-56 rounded-full bg-white/10 blur-3xl"></div>
		<div class="relative flex flex-wrap items-end justify-between gap-5">
			<div class="min-w-0 max-w-2xl">
				<a href="/platform/tenants" class="inline-flex items-center gap-1 text-[11px] font-bold text-white/70 transition hover:text-white">
					<svg viewBox="0 0 24 24" class="size-3.5" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="m15 18-6-6 6-6" /></svg>
					{tx('pf.gw.back', 'Kembali ke tenant')}
				</a>
				<p class="mt-3 text-[10px] font-bold uppercase tracking-[.25em] text-white/60">DEWA · {tx('pf.gw.kicker', 'Gateway pembayaran')}</p>
				<h1 class="mt-1 truncate font-display text-3xl font-extrabold">{data.name || tx('pf.gw.title', 'Akun gateway')}</h1>
				<p class="mt-2 max-w-xl text-[13px] leading-relaxed text-white/75">
					{tx('pf.gw.lead', 'Hubungkan merchant ini ke akun gateway (mis. DOKU) agar bisa membuat QRIS. Tanpa akun yang Aktif, pembuatan QRIS ditolak.')}
				</p>
				<p class="mt-3 flex flex-wrap items-center gap-2 text-[12px] text-white/75">
					<code class="rounded-md bg-white/10 px-2 py-0.5 font-mono text-[11px]">{data.id}</code>
					<button type="button" onclick={copyId} class="rounded-md px-1.5 py-0.5 text-[10px] font-bold text-emas transition hover:bg-white/10">
						{copied ? tx('pf.tn.copied', 'Tersalin ✓') : tx('pf.tn.copyShort', 'Salin')}
					</button>
				</p>
			</div>
			{#if serverEnv}
				<div class="max-w-xs rounded-2xl border border-white/20 bg-white/10 px-4 py-3 backdrop-blur">
					<p class="text-[10px] font-bold uppercase tracking-wider text-white/60">{tx('pf.gw.serverEnv', 'Environment server')}</p>
					<p class="mt-0.5 flex items-center gap-2 font-display text-lg font-extrabold capitalize">
						<span class="size-2.5 rounded-full {serverEnv === 'production' ? 'bg-emerald-400' : 'bg-amber-300'} shadow-[0_0_0_4px_rgba(255,255,255,.12)]"></span>
						{serverEnv}
					</p>
					<p class="mt-1 text-[11px] leading-snug text-white/65">
						{tx('pf.gw.serverEnvHint', 'Server ini hanya memakai akun environment ini. Akun environment lain tersimpan tetapi tidak dipakai dan tidak bisa diuji dari sini.')}
					</p>
				</div>
			{/if}
		</div>
	</section>

	{#if data.error}
		<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{data.error}</p>
	{:else if data.overview}
		<!-- status kesiapan -->
		{#if readyHere}
			<div class="flex items-start gap-3 rounded-2xl border border-emerald-300 bg-emerald-50 px-4 py-3 text-emerald-900" role="status">
				<svg viewBox="0 0 24 24" class="mt-0.5 size-5 shrink-0" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5" /></svg>
				<p class="text-xs font-semibold leading-relaxed">{tx('pf.gw.readyYes', 'Merchant ini siap membuat QRIS di environment')} <b>{serverEnv}</b>. {tx('pf.gw.readyTail', 'Jalankan Tes merchant untuk memastikan Merchant ID diterima gateway.')}</p>
			</div>
		{:else}
			<div class="flex items-start gap-3 rounded-2xl border border-amber-300 bg-amber-50 px-4 py-3 text-amber-900" role="status">
				<svg viewBox="0 0 24 24" class="mt-0.5 size-5 shrink-0" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 9v4m0 4h.01M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z" /></svg>
				<p class="text-xs font-semibold leading-relaxed">
					{tx('pf.gw.warnNone', 'Belum ada akun gateway aktif untuk environment')} <b>{serverEnv}</b>.
					{tx('pf.gw.warnTail', 'Merchant ini belum bisa membuat QRIS (error PAYMENT_METHOD_UNAVAILABLE). Isi Merchant ID dan set status Aktif di bawah.')}
				</p>
			</div>
		{/if}

		<!-- ringkasan -->
		<section class="grid grid-cols-2 gap-3 lg:grid-cols-4">
			{#each [
				{ label: tx('pf.gw.stTotal', 'Akun terdaftar'), n: accounts.length, cls: 'from-merah to-merah-tua', icon: 'M3 7h18M3 12h18M3 17h18' },
				{ label: STATUS.active.label, n: count('active'), cls: STATUS.active.bar, icon: 'm5 12 5 5L20 7' },
				{ label: STATUS.pending_confirmation.label, n: count('pending_confirmation'), cls: STATUS.pending_confirmation.bar, icon: 'M12 6v6l4 2M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20Z' },
				{ label: STATUS.suspended.label, n: count('suspended'), cls: STATUS.suspended.bar, icon: 'M12 9v4m0 4h.01M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z' }
			] as s (s.label)}
				<div class="card flex items-center gap-3.5 p-4">
					<span class="grid size-11 shrink-0 place-items-center rounded-2xl bg-linear-to-br {s.cls} text-white shadow-sm">
						<svg viewBox="0 0 24 24" class="size-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d={s.icon} /></svg>
					</span>
					<span class="min-w-0">
						<span class="block font-display text-2xl font-extrabold leading-none tabular-nums">{s.n}</span>
						<span class="muted mt-1 block truncate text-[11px] font-semibold">{s.label}</span>
					</span>
				</div>
			{/each}
		</section>

		<!-- panduan istilah -->
		<details class="card group overflow-hidden" open={accounts.length === 0}>
			<summary class="flex cursor-pointer list-none items-center justify-between gap-3 p-4 text-sm font-extrabold">
				<span class="flex items-center gap-2.5">
					<span class="grid size-8 place-items-center rounded-xl bg-linear-to-br from-sky-500 to-indigo-600 text-white">
						<svg viewBox="0 0 24 24" class="size-4" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10" /><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 2-3 4M12 17h.01" /></svg>
					</span>
					{tx('pf.gw.guideTitle', 'Panduan: apa itu Merchant ID, environment, dan status?')}
				</span>
				<svg viewBox="0 0 24 24" class="size-4 shrink-0 transition group-open:rotate-180" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6" /></svg>
			</summary>
			<div class="grid gap-4 border-t border-ink/10 p-5 text-xs leading-relaxed lg:grid-cols-3">
				<div class="rounded-2xl bg-sky-500/[.07] p-4">
					<h3 class="text-[13px] font-extrabold text-sky-800">Merchant ID di gateway</h3>
					<p class="mt-1.5">
						Nomor identitas <b>toko ini</b> di gateway, dipakai gateway untuk tahu QR dibuat atas nama merchant mana. Untuk DOKU ini
						<b>Merchant ID</b> (angka, mis. <code class="font-mono">111214</code>), diterbitkan DOKU saat merchant didaftarkan.
					</p>
					<p class="mt-2 rounded-lg bg-surface px-2.5 py-2">
						<b>Bukan Client ID.</b> Client ID (<code class="font-mono">BRN-…</code>) milik akun EraPay ke DOKU, sudah ada di konfigurasi server (<code class="font-mono">DOKU_CLIENT_ID</code>) dan sama untuk semua merchant. Salah mengisi Client ID di sini membuat DOKU menjawab error 500.
					</p>
					<p class="mt-2"><b>Dapatnya dari:</b> dashboard DOKU (atau pihak DOKU yang mendaftarkan merchant) untuk environment yang sesuai; sandbox dan production berbeda. Jika ragu, konfirmasi ke DOKU sebelum mengaktifkan.</p>
				</div>
				<div class="rounded-2xl bg-amber-500/[.09] p-4">
					<h3 class="text-[13px] font-extrabold text-amber-800">Environment</h3>
					<ul class="mt-1.5 space-y-1.5">
						<li><b>Sandbox</b>: uji coba, tidak ada uang sungguhan. Merchant ID-nya khusus sandbox.</li>
						<li><b>Production</b>: transaksi sungguhan. Merchant ID-nya berbeda dari sandbox.</li>
					</ul>
					<p class="mt-2 rounded-lg bg-surface px-2.5 py-2">
						Server hanya memakai akun yang environment-nya sama dengan environment server (<b class="capitalize">{serverEnv || '—'}</b>). Mengisi akun environment lain tidak berpengaruh sampai server dipindah ke environment itu.
					</p>
				</div>
				<div class="rounded-2xl bg-emerald-500/[.08] p-4">
					<h3 class="text-[13px] font-extrabold text-emerald-800">Status & tes</h3>
					<ul class="mt-1.5 space-y-1.5">
						{#each Object.values(STATUS) as s (s.label)}
							<li class="flex gap-2"><span class="mt-1 size-2 shrink-0 rounded-full {s.dot}"></span><span><b>{s.label}</b>: {s.desc}</span></li>
						{/each}
					</ul>
					<p class="mt-2"><b>Tes kredensial</b>: memeriksa koneksi server ke gateway saja. <b>Tes merchant</b>: membuat QR uji Rp1.000 lalu langsung menutupnya, jadi tidak ada uang bergerak.</p>
				</div>
			</div>
		</details>

		<!-- kartu per gateway -->
		{#each gateways as g (g)}
			<section class="card overflow-hidden">
				<div class="h-1 bg-linear-to-r {GW_TONE[g] ?? 'from-slate-400 to-slate-600'}"></div>
				<div class="flex flex-wrap items-center gap-3.5 p-5">
					<span class="grid size-12 shrink-0 place-items-center rounded-2xl bg-linear-to-br {GW_TONE[g] ?? 'from-slate-400 to-slate-600'} font-display text-lg font-extrabold uppercase text-white shadow-sm">{g.slice(0, 1)}</span>
					<div class="min-w-0 flex-1">
						<h2 class="font-display text-lg font-extrabold uppercase">{g}</h2>
						<p class="muted text-xs">{tx('pf.gw.cap', 'Metode pembayaran')}: <span class="pill bg-ink/10 text-ink/70">QRIS</span></p>
					</div>
				</div>

				<div class="grid gap-4 border-t border-ink/10 p-5 lg:grid-cols-2">
					{#each ENVS as e (e)}
						{@const a = find(g, e)}
						{@const st = a ? STATUS[a.activation_status] : null}
						{@const key = `${g}/${e}`}
						<div class="overflow-hidden rounded-2xl border border-ink/10 bg-ink/[.025]">
							<div class="flex items-center justify-between gap-2 px-4 pt-4">
								<div class="flex items-center gap-2">
									<span class="size-2.5 rounded-full {st ? st.dot : 'bg-ink/25'}"></span>
									<h3 class="text-sm font-extrabold capitalize">{e}</h3>
									{#if e === serverEnv}<span class="pill bg-merah/10 text-merah" title={tx('pf.gw.thisServerHint', 'Environment yang sedang dipakai server ini')}>{tx('pf.gw.thisServer', 'Dipakai server ini')}</span>{/if}
								</div>
								{#if st}<span class="pill {st.pill}">{st.label}</span>{:else}<span class="pill bg-ink/10 text-ink/60">{tx('pf.gw.notSet', 'Belum diatur')}</span>{/if}
							</div>

							<dl class="mt-3 grid grid-cols-2 gap-3 px-4 text-xs">
								<div class="min-w-0">
									<dt class="muted text-[10px] font-bold uppercase tracking-wider" title="Nomor identitas toko ini di gateway">Merchant ID</dt>
									<dd class="mt-0.5 truncate font-mono text-[13px] font-bold">{a?.gateway_ref ?? '—'}</dd>
								</div>
								<div>
									<dt class="muted text-[10px] font-bold uppercase tracking-wider">{tx('pf.gw.created', 'Dibuat')}</dt>
									<dd class="mt-0.5 font-semibold">{a ? fmt(a.created_at) : '—'}</dd>
								</div>
							</dl>
							<p class="muted mt-2 px-4 text-[11px] leading-snug">
								{#if !a}
									{tx('pf.gw.tileEmpty', 'Belum ada akun di environment ini. Klik Konfigurasi dan isi Merchant ID dari dashboard gateway.')}
								{:else if !a.gateway_ref}
									{tx('pf.gw.tileNoRef', 'Merchant ID belum diisi, jadi akun ini belum bisa dipakai.')}
								{:else if a.activation_status === 'active'}
									{tx('pf.gw.tileActive', 'Akun siap dipakai membuat QRIS.')}
								{:else}
									{STATUS[a.activation_status].desc}
								{/if}
							</p>

							<div class="mt-4 flex flex-wrap gap-2 px-4 pb-4">
								<button type="button" onclick={() => edit(g, e)} class="rounded-full border border-ink/25 px-4 py-1.5 text-xs font-bold transition hover:bg-ink hover:text-surface">
									{a ? tx('pf.gw.edit', 'Ubah') : tx('pf.gw.configure', 'Konfigurasi')}
								</button>
								{#if e === serverEnv}
									<form
										method="POST"
										action="?/test"
										use:enhance={() => {
											testing = key;
											return async ({ update }) => {
												await update({ reset: false });
												testing = null;
											};
										}}
										class="contents"
									>
										<input type="hidden" name="gateway" value={g} />
										<input type="hidden" name="environment" value={e} />
										<button
											name="deep"
											value="0"
											disabled={testing !== null}
											title={tx('pf.gw.testCredHint', 'Memeriksa apakah server bisa meminta token ke gateway. Tidak mengirim data merchant.')}
											class="rounded-full border border-sky-500/40 bg-sky-500/10 px-4 py-1.5 text-xs font-bold text-sky-700 transition hover:bg-sky-500/20 disabled:opacity-50"
										>
											{tx('pf.gw.testCred', 'Tes kredensial')}
										</button>
										<button
											name="deep"
											value="1"
											disabled={testing !== null || !a?.gateway_ref}
											title={a?.gateway_ref
												? tx('pf.gw.testDeepHint', 'Membuat QR uji Rp1.000 di gateway lalu langsung menutupnya. Tidak ada uang bergerak.')
												: tx('pf.gw.testDeepNeedRef', 'Isi Merchant ID dulu.')}
											class="inline-flex items-center gap-1.5 rounded-full bg-merah px-4 py-1.5 text-xs font-bold text-white transition hover:bg-merah-tua disabled:opacity-50"
										>
											{#if testing === key}
												<svg viewBox="0 0 24 24" class="size-3.5 animate-spin" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round"><path d="M12 3a9 9 0 1 0 9 9" /></svg>
												{tx('pf.gw.testing', 'Menguji…')}
											{:else}
												{tx('pf.gw.testDeep', 'Tes merchant (QR uji)')}
											{/if}
										</button>
									</form>
								{/if}
							</div>

							{#if e !== serverEnv}
								<p class="muted border-t border-ink/10 px-4 py-2.5 text-[11px]">{tx('pf.gw.otherEnv', 'Tidak bisa diuji dari server ini karena server berjalan di environment lain.')}</p>
							{/if}

							{#if form?.tested === key}
								<div class="border-t border-ink/10 bg-surface px-4 py-4" aria-live="polite">
									{#if form.testError}
										<p class="text-xs font-semibold text-red-700" role="alert">{form.testError}</p>
									{:else if form.test}
										<ol>
											{#each form.test.steps as s, i (s.name)}
												<li class="relative flex gap-3 pb-4 last:pb-0">
													{#if i < form.test.steps.length - 1}<span class="absolute left-[11px] top-6 h-[calc(100%-1.25rem)] w-px bg-ink/15"></span>{/if}
													<span class="relative grid size-6 shrink-0 place-items-center rounded-full text-[11px] font-extrabold text-white {s.ok ? 'bg-emerald-500' : 'bg-red-500'}">{s.ok ? '✓' : '✕'}</span>
													<div class="min-w-0">
														<p class="text-xs font-extrabold">{STEP[s.name]?.title ?? s.name}</p>
														<p class="muted text-[11px]">{STEP[s.name]?.desc ?? ''}</p>
														{#if s.message}
															<p class="mt-1 rounded-lg bg-red-500/10 px-2.5 py-1.5 text-[11px] font-semibold text-red-800">{s.message}{s.code ? ` (${s.code})` : ''}</p>
														{/if}
													</div>
												</li>
											{/each}
										</ol>
										<p class="mt-3 rounded-xl px-3 py-2 text-xs font-extrabold {form.test.ok ? 'bg-emerald-500/15 text-emerald-800' : 'bg-red-500/15 text-red-800'}">
											{form.test.ok ? tx('pf.gw.testOk', 'Koneksi berhasil.') : tx('pf.gw.testFail', 'Koneksi gagal.')}
											{#if form.test.ok && !form.test.deep}<span class="font-semibold">{tx('pf.gw.testShallowNote', 'Merchant ID belum diuji; jalankan Tes merchant.')}</span>{/if}
										</p>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				</div>
			</section>
		{/each}

		<!-- form konfigurasi -->
		<section class="card overflow-hidden" bind:this={formEl}>
			<div class="h-1 bg-linear-to-r from-merah to-emas"></div>
			<div class="p-5 sm:p-6">
				<h2 class="font-display text-lg font-extrabold">{tx('pf.gw.setTitle', 'Konfigurasi akun')}</h2>
				<p class="muted mt-1 max-w-2xl text-xs leading-relaxed">
					{tx('pf.gw.setHint', 'Pilih gateway dan environment, isi Merchant ID dari dashboard gateway, lalu set status. Setiap perubahan dicatat di audit beserta alasannya.')}
				</p>
				<form
					method="POST"
					action="?/save"
					autocomplete="off"
					use:enhance={() => {
						saving = true;
						return async ({ update }) => {
							await update({ reset: false });
							saving = false;
						};
					}}
					class="mt-5 grid gap-4 sm:grid-cols-2"
				>
					<div>
						<label class="lbl mb-1.5" for="gw-gateway">{tx('pf.gw.fGateway', 'Gateway')}</label>
						<Combobox id="gw-gateway" name="gateway" required bind:value={fGateway} options={gateways.map((g) => ({ value: g, label: g.toUpperCase() }))} />
						<p class="muted mt-1.5 text-[11px] leading-snug">{tx('pf.gw.fGatewayHint', 'Penyedia pembayaran yang dipakai merchant.')}</p>
					</div>
					<div>
						<label class="lbl mb-1.5" for="gw-env">{tx('pf.gw.fEnv', 'Environment')}</label>
						<Combobox id="gw-env" name="environment" required bind:value={fEnv} options={envOptions} />
						<p class="muted mt-1.5 text-[11px] leading-snug">
							{fEnv === 'sandbox'
								? tx('pf.gw.fEnvSandbox', 'Sandbox: uji coba tanpa uang sungguhan.')
								: tx('pf.gw.fEnvProd', 'Production: transaksi sungguhan. Pastikan Merchant ID dari akun production.')}
							{#if serverEnv && fEnv !== serverEnv}
								<span class="font-bold text-amber-700">{tx('pf.gw.fEnvMismatch', 'Server ini berjalan di')} {serverEnv}, {tx('pf.gw.fEnvMismatchTail', 'jadi akun ini tidak akan dipakai sekarang.')}</span>
							{/if}
						</p>
					</div>
					<div>
						<label class="lbl mb-1.5" for="gw-ref">{tx('pf.gw.fRef', 'Merchant ID di gateway')}</label>
						<input id="gw-ref" name="gateway_ref" bind:value={fRef} maxlength="64" pattern={'[A-Za-z0-9._\\-]*'} class="inp font-mono" placeholder="mis. 111214" autocomplete="off" />
						<p class="muted mt-1.5 text-[11px] leading-snug">
							{tx('pf.gw.fRefHint', 'Nomor identitas toko ini di gateway (DOKU: Merchant ID, berupa angka). Salin dari dashboard DOKU untuk environment yang dipilih. Bukan Client ID (BRN-…). Huruf, angka, titik, garis bawah, dan strip saja.')}
						</p>
					</div>
					<div>
						<label class="lbl mb-1.5" for="gw-status">{tx('pf.gw.fStatus', 'Status')}</label>
						<Combobox id="gw-status" name="status" required bind:value={fStatus} options={statusOptions} />
						<p class="muted mt-1.5 text-[11px] leading-snug">{STATUS[fStatus]?.desc ?? ''}</p>
					</div>
					<div class="sm:col-span-2">
						<label class="lbl mb-1.5" for="gw-note">{tx('pf.gw.fNote', 'Alasan (wajib, masuk audit)')}</label>
						<input id="gw-note" name="note" required minlength="3" maxlength="500" class="inp" placeholder={tx('pf.gw.notePh', 'mis. Merchant ID dikonfirmasi di dashboard DOKU')} />
						<p class="muted mt-1.5 text-[11px] leading-snug">{tx('pf.gw.fNoteHint', 'Tersimpan di log audit bersama nama Anda dan waktu perubahan (minimal 3 karakter).')}</p>
					</div>
					<div class="flex flex-wrap items-center gap-3 sm:col-span-2">
						<button disabled={saving} class="rounded-full bg-merah px-6 py-2.5 text-xs font-bold text-white shadow-sm transition hover:bg-merah-tua disabled:opacity-50">
							{saving ? tx('pf.gw.saving', 'Menyimpan…') : tx('pf.gw.save', 'Simpan')}
						</button>
						{#if form?.saveError}<p class="text-xs font-semibold text-red-700" role="alert">{form.saveError}</p>{/if}
						{#if form?.saved}<p class="text-xs font-semibold text-emerald-700">{tx('pf.gw.saved', 'Tersimpan. Jalankan Tes merchant untuk memastikan akun diterima gateway.')}</p>{/if}
					</div>
				</form>
			</div>
		</section>
	{/if}
</div>
