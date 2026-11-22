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
			desc: 'Merchant boleh membuat QRIS lewat akun ini. Hanya pilih ini setelah ID toko dipastikan benar.',
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
		credentials: { title: 'Kredensial gateway', desc: 'Kredensial gateway di server (disimpan di panel Gateways atau env) diterima oleh gateway' },
		merchant: { title: 'ID toko di gateway', desc: 'Gateway menerima pembuatan QR uji Rp1.000 untuk ID toko ini' },
		cleanup: { title: 'Tutup QR uji', desc: 'QR uji langsung dikedaluwarsakan agar tidak bisa dibayar' }
	};
	const statusOptions = Object.entries(STATUS).map(([value, s]) => ({ value, label: s.label }));
	const envOptions = [
		{ value: 'sandbox', label: 'Sandbox (uji coba)' },
		{ value: 'production', label: 'Production (uang sungguhan)' }
	];

	type Acc = { id: string; gateway: string; environment: Env; gateway_ref?: string; activation_status: string; created_at: string };
	const serverEnv = $derived((data.overview?.server_environment ?? '') as Env | '');
	// Server production juga melayani sandbox (adapter terpisah); server sandbox tidak bisa menguji production.
	const served = (e: string) => e === serverEnv || (e === 'sandbox' && serverEnv === 'production');
	const gateways = $derived(data.overview?.gateways ?? []);
	// Gateway yang dikontrak sekali untuk platform: tidak butuh ID toko per merchant.
	const isPlatform = (g: string) => (data.overview?.platform_gateways ?? []).includes(g);
	const accounts = $derived((data.overview?.accounts ?? []) as Acc[]);
	const find = (g: string, e: Env) => accounts.find((a) => a.gateway === g && a.environment === e);
	const readyHere = $derived(accounts.some((a) => a.environment === serverEnv && a.activation_status === 'active' && (a.gateway_ref || isPlatform(a.gateway))));

	// Jalur pembayaran tenant (menimpa default platform).
	const routing = $derived(data.routing);
	const routeOptions = $derived([
		{ value: '', label: `Ikut default platform${routing?.platform_gateway ? ` (${routing.platform_gateway.toUpperCase()})` : ''}` },
		...(routing?.options ?? []).map((o) => ({ value: o.code, label: o.code.toUpperCase() }))
	]);
	let rGateway = $state('');
	let rFallback = $state(false);
	let rSeeded = false;
	let routing_saving = $state(false);
	$effect(() => {
		if (rSeeded || !routing) return;
		rSeeded = true;
		rGateway = routing.gateway;
		rFallback = routing.fallback;
	});

	let testing = $state<string | null>(null);
	let saving = $state(false);
	let copied = $state(false);
	let fGateway = $state('');
	let fEnv = $state<string>('production');
	let fStatus = $state('active');
	let fRef = $state('');
	let formEl = $state<HTMLElement | null>(null);
	let formOpen = $state(false);
	let seeded = false;
	$effect(() => {
		if (seeded || !data.overview) return;
		seeded = true;
		fGateway = data.overview.gateways[0] ?? '';
		fEnv = data.overview.server_environment === 'sandbox' ? 'sandbox' : 'production';
	});

	function edit(g: string, e: Env) {
		const a = find(g, e);
		fGateway = g;
		fEnv = e;
		fStatus = a?.activation_status ?? 'active';
		fRef = a?.gateway_ref ?? '';
		formOpen = true;
		setTimeout(() => formEl?.scrollIntoView({ behavior: 'smooth', block: 'center' }), 0);
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
	// Baris tabel: akun yang sudah ada + environment server (supaya terlihat jelas bila belum diatur).
	const rows = $derived(gateways.flatMap((g) => ENVS.filter((e) => find(g, e) || e === serverEnv).map((e) => ({ g, e, a: find(g, e) }))));
</script>

<svelte:head><title>{tx('pf.gw.pageTitle', 'Akun gateway')}</title></svelte:head>

<div class="space-y-5">
	<header class="flex flex-wrap items-start justify-between gap-3">
		<div class="min-w-0">
			<a href="/platform/tenants" class="text-[11px] font-bold text-merah hover:underline">← {tx('pf.gw.back', 'Kembali ke tenant')}</a>
			<h1 class="mt-1 truncate font-display text-2xl font-extrabold">{data.name || tx('pf.gw.title', 'Akun gateway')}</h1>
			<p class="muted mt-0.5 flex items-center gap-2 text-xs">
				<code class="font-mono text-[11px]">{data.id}</code>
				<button type="button" onclick={copyId} class="font-bold text-merah hover:underline">{copied ? tx('pf.tn.copied', 'Tersalin ✓') : tx('pf.tn.copyShort', 'Salin')}</button>
			</p>
		</div>
		{#if serverEnv}
			<span class="pill bg-ink/10 text-ink/70" title={tx('pf.gw.serverEnvHint', 'Server hanya memakai akun environment ini.')}>Server: <b class="capitalize">{serverEnv}</b></span>
		{/if}
	</header>

	{#if data.error}
		<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{data.error}</p>
	{:else if data.overview}
		<p class="rounded-xl px-4 py-3 text-xs font-semibold {readyHere ? 'bg-emerald-50 text-emerald-900' : 'bg-amber-50 text-amber-900'}" role="status">
			{#if readyHere}
				✓ Siap membuat QRIS di <b>{serverEnv}</b>. Jalankan “Tes” untuk memastikan ID toko diterima gateway.
			{:else}
				! Belum ada akun aktif di <b>{serverEnv}</b>, QRIS ditolak (PAYMENT_METHOD_UNAVAILABLE). Klik Atur, isi ID toko, set status Aktif.
			{/if}
		</p>

		<section class="card overflow-hidden">
			<div class="flex items-center justify-between gap-3 border-b border-ink/10 px-4 py-3">
				<h2 class="font-display text-base font-extrabold">Akun gateway</h2>
				<button type="button" onclick={() => { formOpen = !formOpen; if (formOpen) setTimeout(() => formEl?.scrollIntoView({ behavior: 'smooth', block: 'center' }), 0); }} class="rounded-full border border-ink/25 px-3.5 py-1.5 text-xs font-bold transition hover:bg-ink hover:text-surface">
					{formOpen ? 'Tutup form' : '+ Atur akun'}
				</button>
			</div>
			<div class="overflow-x-auto">
				<table class="w-full text-left text-xs">
					<thead class="bg-gading text-[10px] font-bold uppercase tracking-wider text-ink/60">
						<tr><th class="px-4 py-2">Gateway</th><th class="px-4 py-2">Env</th><th class="px-4 py-2" title="Nomor identitas toko ini di gateway">ID toko</th><th class="px-4 py-2">Status</th><th class="px-4 py-2 text-right">Aksi</th></tr>
					</thead>
					<tbody class="divide-y divide-ink/10">
						{#each rows as { g, e, a } (`${g}/${e}`)}
							{@const st = a ? STATUS[a.activation_status] : null}
							{@const key = `${g}/${e}`}
							{@const canDeep = !!a && (!!a.gateway_ref || isPlatform(g))}
							<tr>
								<td class="px-4 py-3 font-extrabold uppercase">{g}</td>
								<td class="px-4 py-3 capitalize">{e}</td>
								<td class="px-4 py-3 font-mono font-bold">{isPlatform(g) ? 'Platform' : (a?.gateway_ref ?? '—')}</td>
								<td class="px-4 py-3">
									{#if st}<span class="pill {st.pill}" title={st.desc}>{st.label}</span>{:else}<span class="pill bg-ink/10 text-ink/60">Belum diatur</span>{/if}
								</td>
								<td class="px-4 py-3">
									<div class="flex flex-wrap justify-end gap-1.5">
										<button type="button" onclick={() => edit(g, e)} class="rounded-full border border-ink/25 px-3 py-1 text-[11px] font-bold transition hover:bg-ink hover:text-surface">{a ? 'Ubah' : 'Atur'}</button>
										{#if served(e)}
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
												<button name="deep" value="0" disabled={testing !== null} title="Cek koneksi server ke gateway (tanpa data merchant)" class="rounded-full border border-sky-500/40 bg-sky-500/10 px-3 py-1 text-[11px] font-bold text-sky-700 transition hover:bg-sky-500/20 disabled:opacity-50">Tes koneksi</button>
												<button name="deep" value="1" disabled={testing !== null || !canDeep} title={canDeep ? 'Buat QR uji Rp1.000 lalu langsung ditutup, tidak ada uang bergerak' : 'Isi ID toko dulu'} class="rounded-full bg-merah px-3 py-1 text-[11px] font-bold text-white transition hover:bg-merah-tua disabled:opacity-50">
													{testing === key ? 'Menguji…' : 'Tes merchant'}
												</button>
											</form>
										{/if}
									</div>
								</td>
							</tr>
							{#if form?.tested === key}
								<tr>
									<td colspan="5" class="bg-surface px-4 py-3" aria-live="polite">
										{#if form.testError}
											<p class="font-semibold text-red-700" role="alert">{form.testError}</p>
										{:else if form.test}
											<ul class="space-y-1">
												{#each form.test.steps as s (s.name)}
													<li class="flex gap-2">
														<span class="font-extrabold {s.ok ? 'text-emerald-600' : 'text-red-600'}">{s.ok ? '✓' : '✕'}</span>
														<span><b>{STEP[s.name]?.title ?? s.name}</b>{#if s.message}<span class="text-red-700"> — {s.message}{s.code ? ` (${s.code})` : ''}</span>{/if}</span>
													</li>
												{/each}
											</ul>
											<p class="mt-2 font-extrabold {form.test.ok ? 'text-emerald-700' : 'text-red-700'}">
												{form.test.ok ? 'Koneksi berhasil.' : 'Koneksi gagal.'}
												{#if form.test.ok && !form.test.deep}<span class="font-semibold text-ink/70">ID toko belum diuji; jalankan Tes merchant.</span>{/if}
											</p>
										{/if}
									</td>
								</tr>
							{/if}
						{/each}
					</tbody>
				</table>
			</div>
		</section>

		<!-- form: tampil hanya saat Atur/Ubah -->
		{#if formOpen}
			<section class="card p-5" bind:this={formEl}>
				<h2 class="font-display text-base font-extrabold">Atur akun</h2>
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
					class="mt-4 grid gap-4 sm:grid-cols-2"
				>
					<div>
						<label class="lbl mb-1.5" for="gw-gateway">Gateway</label>
						<Combobox id="gw-gateway" name="gateway" required bind:value={fGateway} options={gateways.map((g) => ({ value: g, label: g.toUpperCase() }))} />
					</div>
					<div>
						<label class="lbl mb-1.5" for="gw-env">Environment</label>
						<Combobox id="gw-env" name="environment" required bind:value={fEnv} options={envOptions} />
						{#if serverEnv && fEnv !== serverEnv}
							<p class="mt-1.5 text-[11px] font-bold text-amber-700">Server berjalan di {serverEnv}, akun ini belum dipakai.</p>
						{/if}
					</div>
					{#if isPlatform(fGateway)}
						<p class="rounded-xl bg-sky-500/[.07] p-3 text-[11px] leading-snug">Memakai kredensial platform (menu Gateways). Cukup set status.</p>
					{:else}
						<div>
							<label class="lbl mb-1.5" for="gw-ref">ID toko di gateway</label>
							<input id="gw-ref" name="gateway_ref" bind:value={fRef} maxlength="64" pattern={'[A-Za-z0-9._\\-]*'} class="inp font-mono" placeholder="mis. 111214" autocomplete="off" />
							<p class="muted mt-1.5 text-[11px] leading-snug">DOKU: Merchant ID (angka) dari dashboard gateway, sesuai environment. Bukan kredensial EraPay.</p>
						</div>
					{/if}
					<div>
						<label class="lbl mb-1.5" for="gw-status">Status</label>
						<Combobox id="gw-status" name="status" required bind:value={fStatus} options={statusOptions} />
						<p class="muted mt-1.5 text-[11px] leading-snug">{STATUS[fStatus]?.desc ?? ''}</p>
					</div>
					<div class="sm:col-span-2">
						<label class="lbl mb-1.5" for="gw-note">Alasan (wajib, masuk audit)</label>
						<input id="gw-note" name="note" required minlength="3" maxlength="500" class="inp" placeholder="mis. ID toko dikonfirmasi di dashboard gateway" />
					</div>
					<div class="flex flex-wrap items-center gap-3 sm:col-span-2">
						<button disabled={saving} class="rounded-full bg-merah px-6 py-2 text-xs font-bold text-white shadow-sm transition hover:bg-merah-tua disabled:opacity-50">{saving ? 'Menyimpan…' : 'Simpan'}</button>
						{#if form?.saveError}<p class="text-xs font-semibold text-red-700" role="alert">{form.saveError}</p>{/if}
						{#if form?.saved}<p class="text-xs font-semibold text-emerald-700">Tersimpan. Jalankan Tes merchant untuk memastikan.</p>{/if}
					</div>
				</form>
			</section>
		{/if}

		<!-- pengaturan lanjutan: jarang diubah -->
		{#if routing || data.exact !== null}
			<details class="card overflow-hidden" open={!!form?.routeError || !!form?.exactError || !!form?.routed || !!form?.exactSaved}>
				<summary class="cursor-pointer list-none px-4 py-3 font-display text-base font-extrabold">Pengaturan lanjutan</summary>
				<div class="space-y-6 border-t border-ink/10 p-5">
					{#if routing}
						<form
							method="POST"
							action="?/route"
							autocomplete="off"
							use:enhance={() => {
								routing_saving = true;
								return async ({ update }) => {
									await update({ reset: false });
									routing_saving = false;
									rSeeded = false;
								};
							}}
							class="grid gap-4 sm:grid-cols-2"
						>
							<input type="hidden" name="environment" value={routing.server_environment} />
							<input type="hidden" name="version" value={routing.version} />
							<input type="hidden" name="fallback" value={rFallback ? '1' : '0'} />
							<div class="sm:col-span-2">
								<h3 class="text-sm font-extrabold">Jalur pembayaran</h3>
								<p class="muted text-[11px]">Gateway tujuan pembayaran baru ({routing.server_environment}). Kosong = ikut default platform. Pembayaran lama tidak berubah.</p>
							</div>
							<div>
								<label class="lbl mb-1.5" for="rt-gateway">Jalur ke</label>
								<Combobox id="rt-gateway" name="gateway" bind:value={rGateway} options={routeOptions} />
							</div>
							<label class="flex items-start gap-2 self-end rounded-xl bg-amber-500/[.07] p-3 text-[11px] leading-snug">
								<input type="checkbox" bind:checked={rFallback} class="mt-0.5" />
								<span><b>Boleh pindah ke jalur biasa bila gagal.</b> Matikan untuk gateway langsung (QRISGate): lebih baik ditolak daripada diam-diam pindah jalur.</span>
							</label>
							<div class="sm:col-span-2">
								<label class="lbl mb-1.5" for="rt-note">Alasan (wajib, masuk audit)</label>
								<input id="rt-note" name="note" required minlength="3" maxlength="500" class="inp" placeholder="mis. tenant memakai QRISGate ke rekening sendiri" />
							</div>
							<div class="flex flex-wrap items-center gap-3 sm:col-span-2">
								<button disabled={routing_saving} class="rounded-full bg-merah px-6 py-2 text-xs font-bold text-white shadow-sm transition hover:bg-merah-tua disabled:opacity-50">{routing_saving ? 'Menyimpan…' : 'Simpan jalur'}</button>
								{#if form?.routeError}<p class="text-xs font-semibold text-red-700" role="alert">{form.routeError}</p>{/if}
								{#if form?.routed}<p class="text-xs font-semibold text-emerald-700">Tersimpan. Berlaku untuk pembayaran berikutnya.</p>{/if}
							</div>
						</form>
					{/if}

					{#if data.exact !== null}
						<form method="POST" action="?/exact" autocomplete="off" use:enhance class="grid gap-3 sm:grid-cols-2 {routing ? 'border-t border-ink/10 pt-6' : ''}">
							<input type="hidden" name="enabled" value={data.exact ? '0' : '1'} />
							<div class="sm:col-span-2">
								<h3 class="text-sm font-extrabold">QRISGate: nominal persis <span class="pill ml-1 {data.exact ? 'bg-emerald-500/15 text-emerald-700' : 'bg-ink/10 text-ink/60'}">{data.exact ? 'Aktif' : 'Nonaktif'}</span></h3>
								<p class="muted text-[11px]">Bawaan: harga + kode unik 1-99. Nyalakan hanya atas permintaan tenant. Risiko: transfer lain dengan nominal sama persis bisa ikut melunasi transaksi yang menunggu.</p>
							</div>
							<input name="note" required minlength="3" maxlength="500" class="inp sm:col-span-2" placeholder="Alasan (mis. permintaan tenant lewat WA tgl …)" />
							<div class="flex flex-wrap items-center gap-3 sm:col-span-2">
								<button class="rounded-full bg-merah px-6 py-2 text-xs font-bold text-white shadow-sm transition hover:bg-merah-tua">{data.exact ? 'Matikan' : 'Nyalakan'}</button>
								{#if form?.exactError}<p class="text-xs font-semibold text-red-700" role="alert">{form.exactError}</p>{/if}
								{#if form?.exactSaved}<p class="text-xs font-semibold text-emerald-700">Tersimpan. Berlaku untuk pembayaran berikutnya.</p>{/if}
							</div>
						</form>
					{/if}
				</div>
			</details>
		{/if}
	{/if}
</div>
