<script lang="ts">
	import { enhance } from '$app/forms';
	import { invalidateAll } from '$app/navigation';
	import { getI18n } from '../../../../lib/i18n/i18n.svelte';
	import GatewayLogo from '../../../../lib/components/GatewayLogo.svelte';
	import type { CredEnv, CredGateway, CredStep } from './+page.server';

	let { data, form } = $props();
	const { tx } = getI18n();

	const STEP: Record<string, string> = { credentials: 'Kredensial gateway (token berhasil diminta)' };
	const fmt = (s?: string) => (s ? new Date(s).toLocaleString('id-ID', { day: '2-digit', month: 'short', year: 'numeric', hour: '2-digit', minute: '2-digit' }) : '—');

	const fm = $derived(form as { key?: string; error?: string; saved?: boolean; result?: { ok: boolean; applied: boolean; steps: CredStep[] } } | null);
	const serverEnv = $derived(data.overview?.server_environment ?? '');
	// Server production juga melayani sandbox (adapter terpisah); server sandbox tidak bisa menguji production.
	const served = (e: string) => e === serverEnv || (e === 'sandbox' && serverEnv === 'production');
	let busy = $state<string | null>(null);

	const keyOf = (g: CredGateway, e: CredEnv) => `${g.gateway}/${e.environment}`;

	const rf = $derived(form as { key?: string; error?: string; routed?: string; feeSaved?: boolean; apkSaved?: string } | null);
	const routing = $derived(data.routing);
	const LABEL: Record<string, string> = { doku: 'DOKU', ids: 'IDS Payments', finpay: 'FINPAY', xendit: 'Xendit', midtrans: 'Midtrans', dana: 'DANA', qrisgate: 'QRISGate' };
	// Pilihan di form; mengikuti pilihan tersimpan sampai admin mengubahnya.
	let pick = $state('');
	$effect(() => {
		pick = routing?.gateway ?? '';
	});
	// APK dikirim langsung dari browser ke API Go (bukan lewat server web) dengan tiket sekali pakai.
	let apkMsg = $state<{ error?: string; saved?: string } | null>(null);
	async function uploadApk(e: SubmitEvent) {
		e.preventDefault();
		const form = e.currentTarget as HTMLFormElement;
		const fd = new FormData(form);
		const file = fd.get('apk') as File;
		const version = String(fd.get('version') ?? '').trim();
		const note = String(fd.get('note') ?? '').trim();
		if (!file?.size) return void (apkMsg = { error: 'Pilih berkas APK.' });
		if (!/^[0-9A-Za-z][0-9A-Za-z._+-]{0,31}$/.test(version)) return void (apkMsg = { error: 'Versi 1-32 karakter (huruf, angka, . _ + -).' });
		if (file.size > 60 * 1024 * 1024) return void (apkMsg = { error: 'APK lebih dari 60 MB.' });
		busy = 'apk';
		apkMsg = null;
		try {
			const t = await fetch('/platform/gateways/apk-ticket', { method: 'POST' });
			if (!t.ok) throw new Error((await t.json().catch(() => null))?.message ?? 'Sesi berakhir, muat ulang halaman.');
			const { url } = await t.json();
			const r = await fetch(`${url}&${new URLSearchParams({ version, note })}`, { method: 'POST', headers: { 'content-type': 'application/octet-stream' }, body: file });
			const j = await r.json().catch(() => null);
			if (!r.ok) throw new Error(j?.error?.message ?? `Unggah gagal (${r.status}).`);
			apkMsg = { saved: version };
			form.reset();
			await invalidateAll();
		} catch (err) {
			apkMsg = { error: err instanceof Error ? err.message : 'Unggah gagal.' };
		} finally {
			busy = null;
		}
	}
	const mineRoute = $derived(rf?.key === 'route');
	const gwReady = $derived((data.overview?.gateways ?? []).filter((g: CredGateway) => g.environments.some((e: CredEnv) => e.complete)).length);
	const gwTotal = $derived(data.overview?.gateways?.length ?? 0);
	const apkLatest = $derived(data.apkReleases?.[0]);
	const envCls = (e: CredEnv) => (e.complete ? 'bg-emerald-500' : e.configured ? 'bg-amber-500' : 'bg-ink/25');
	const envTxt = (e: CredEnv) => (e.complete ? 'Lengkap' : e.configured ? 'Belum lengkap' : 'Memakai .env');
</script>

<svelte:head><title>{tx('pf.nav.gateways', 'Gateway')}</title></svelte:head>

{#snippet chev()}
	<svg viewBox="0 0 24 24" class="size-5 shrink-0 text-ink/50 transition-transform [details[open]>summary_&]:rotate-180" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg>
{/snippet}

<div class="space-y-5">
	<header class="relative overflow-hidden rounded-2xl bg-linear-to-br from-merah-gelap via-merah-tua to-merah px-5 py-4 text-white shadow-md sm:px-6">
		<div class="pointer-events-none absolute -right-16 -top-20 size-56 rounded-full bg-emas/25 blur-3xl"></div>
		<p class="relative text-[10px] font-bold uppercase tracking-[.25em] text-white/60">DEWA · {tx('pf.gw.kicker', 'Gateway pembayaran')}</p>
		<h1 class="relative mt-0.5 font-display text-2xl font-extrabold">{tx('pf.gc.title', 'Kredensial gateway')}</h1>
		<p class="relative mt-1 max-w-3xl text-xs leading-relaxed text-white/75">
			{tx('pf.gc.lead', 'Kelola Client ID, secret, dan private key gateway dari sini tanpa mengedit .env di server. Nilai disimpan terenkripsi dan tidak pernah ditampilkan lagi setelah disimpan.')}
		</p>
	</header>

	<dl class="grid grid-cols-2 gap-3 lg:grid-cols-4">
		<div class="card px-4 py-3">
			<dt class="text-[10px] font-bold uppercase tracking-wider text-ink/50">Server berjalan di</dt>
			<dd class="mt-0.5 text-base font-extrabold capitalize">{serverEnv || '—'}</dd>
		</div>
		<div class="card px-4 py-3">
			<dt class="text-[10px] font-bold uppercase tracking-wider text-ink/50">Jalur pembayaran</dt>
			<dd class="mt-0.5 truncate text-base font-extrabold">{routing?.gateway ? (LABEL[routing.gateway] ?? routing.gateway) : 'Per merchant'}</dd>
		</div>
		<div class="card px-4 py-3">
			<dt class="text-[10px] font-bold uppercase tracking-wider text-ink/50">Gateway terkonfigurasi</dt>
			<dd class="mt-0.5 text-base font-extrabold">{gwReady}<span class="text-ink/40"> / {gwTotal}</span></dd>
		</div>
		<div class="card px-4 py-3">
			<dt class="text-[10px] font-bold uppercase tracking-wider text-ink/50">APK Listener</dt>
			<dd class="mt-0.5 text-base font-extrabold">{apkLatest ? `v${apkLatest.version_name}` : '—'}</dd>
		</div>
	</dl>

	{#if data.unavailable}
		<p class="rounded-xl border border-amber-300 bg-amber-50 px-4 py-3 text-xs font-semibold leading-relaxed text-amber-900" role="status">
			{tx('pf.gc.off', 'Endpoint kredensial gateway tidak ditemukan di API. Biasanya API belum di-restart setelah update; kalau sudah, cek ERAPAY_ENCRYPTION_KEY di .env server API. Server tetap memakai kredensial dari .env seperti biasa.')}
		</p>
	{:else if data.error}
		<p class="rounded-xl border border-red-300 bg-red-50 px-4 py-3 text-xs font-semibold text-red-800" role="alert">{data.error}</p>
	{/if}

	{#if routing}
		<details class="card overflow-hidden" open={mineRoute}>
			<summary class="flex cursor-pointer list-none items-center gap-3 px-4 py-3.5 transition hover:bg-ink/[.03] sm:px-5 [&::-webkit-details-marker]:hidden">
				<div class="min-w-0 flex-1">
					<h2 class="font-display text-base font-extrabold uppercase">{tx('pf.rt.title', 'Jalur pembayaran')}</h2>
					<p class="mt-0.5 truncate text-[11px] font-semibold text-ink/60">
						{#if routing.gateway}
							{tx('pf.rt.now', 'Dipakai sekarang')}: <b>{LABEL[routing.gateway] ?? routing.gateway}</b> · v{routing.version} · {fmt(routing.updated_at)}
						{:else}
							{tx('pf.rt.none', 'Belum dipilih: tiap merchant memakai akun gateway aktifnya sendiri (biasanya DOKU).')}
						{/if}
					</p>
				</div>
				<span class="pill hidden sm:inline-flex {routing.gateway ? 'bg-emerald-500/15 text-emerald-700' : 'bg-ink/10 text-ink/60'}">{routing.gateway ? 'Terpilih' : 'Belum dipilih'}</span>
				{@render chev()}
			</summary>
			<form
				method="POST"
				action="?/route"
				autocomplete="off"
				use:enhance={() => {
					busy = 'route';
					return async ({ update }) => {
						await update({ reset: false });
						busy = null;
					};
				}}
				class="space-y-3 border-t border-ink/10 p-4 sm:p-5"
			>
				<p class="text-xs leading-relaxed text-ink/70">
					{tx('pf.rt.lead', 'Pilih gateway yang menerima transaksi baru. Berlaku untuk pembayaran berikutnya; transaksi yang sudah dibuat tetap di gateway-nya. Kalau gateway terpilih tidak bisa melayani seorang merchant, transaksi itu memakai akun gateway milik merchant tersebut.')}
				</p>
				<input type="hidden" name="environment" value={routing.server_environment} />
				<input type="hidden" name="version" value={routing.version} />
				<div class="grid gap-2 sm:grid-cols-2 xl:grid-cols-3" role="radiogroup" aria-label={tx('pf.rt.title', 'Jalur pembayaran')}>
					{#each routing.options as o (o.code)}
						<label class="flex cursor-pointer items-start gap-3 rounded-xl border p-3 transition {pick === o.code ? 'border-merah bg-merah/5' : 'border-ink/10 hover:bg-ink/[.03]'} {o.available ? '' : 'cursor-not-allowed opacity-60'}">
							<input type="radio" name="gateway" value={o.code} bind:group={pick} disabled={!o.available} class="mt-1" />
							<span class="min-w-0 flex-1">
								<span class="flex flex-wrap items-center gap-2">
									<GatewayLogo code={o.code} label={LABEL[o.code] ?? o.code} class="h-6" />
									<b class="text-sm">{LABEL[o.code] ?? o.code}</b>
									{#if routing.gateway === o.code}<span class="pill bg-merah/10 text-merah">{tx('pf.rt.current', 'Terpilih')}</span>{/if}
									<span class="pill {o.available ? 'bg-emerald-500/15 text-emerald-700' : 'bg-amber-500/15 text-amber-700'}">{o.available ? tx('pf.rt.ready', 'Siap') : tx('pf.rt.notReady', 'Belum aktif di server')}</span>
								</span>
								<span class="muted mt-1 block text-[11px] leading-snug">
									{#if !o.available}
										{tx('pf.rt.needCreds', 'Simpan lalu Terapkan kredensialnya di bawah dulu.')}
									{:else if o.platform_account}
										{tx('pf.rt.platformAcct', 'Satu akun untuk seluruh platform: merchant yang sudah disetujui tidak perlu akun gateway sendiri.')}
									{:else}
										{tx('pf.rt.merchantAcct', 'Hanya merchant dengan akun gateway aktif yang dilayani; lainnya memakai akunnya sendiri.')}
									{/if}
								</span>
							</span>
						</label>
					{/each}
				</div>
				<div class="grid items-end gap-3 sm:grid-cols-[1fr_auto]">
					<div>
						<label class="lbl mb-1" for="route-note">{tx('pf.gw.fNote', 'Alasan (wajib, masuk audit)')}</label>
						<input id="route-note" name="note" required minlength="3" maxlength="500" class="inp" placeholder="mis. DOKU sedang gangguan" />
					</div>
					<button disabled={busy !== null || !pick || pick === routing.gateway} class="rounded-full bg-merah px-5 py-2.5 text-xs font-bold text-white shadow-sm transition hover:bg-merah-tua disabled:opacity-50">
						{busy === 'route' ? 'Menyimpan…' : tx('pf.rt.save', 'Simpan jalur')}
					</button>
				</div>
				{#if mineRoute && rf?.error}<p class="text-xs font-semibold text-red-700" role="alert">{rf.error}</p>{/if}
				{#if mineRoute && rf?.routed}<p class="text-xs font-semibold text-emerald-700" role="status">{tx('pf.rt.saved', 'Tersimpan. Transaksi berikutnya lewat')} {LABEL[rf.routed] ?? rf.routed}.</p>{/if}
			</form>
		</details>
	{/if}

	{#if data.overview}
		<section class="space-y-2" aria-labelledby="gw-list">
			<h2 id="gw-list" class="px-1 text-[11px] font-bold uppercase tracking-[.2em] text-ink/50">Kredensial per gateway</h2>

			{#each data.overview.gateways as g (g.gateway)}
				<details class="card overflow-hidden" open={fm?.key?.startsWith(`${g.gateway}/`)}>
					<summary class="flex cursor-pointer list-none items-center gap-3 px-4 py-3 transition hover:bg-ink/[.03] sm:px-5 [&::-webkit-details-marker]:hidden">
						<GatewayLogo code={g.gateway} label={g.label} class="h-10" />
						<div class="min-w-0 flex-1">
							<h3 class="font-display text-base font-extrabold uppercase">{g.label}</h3>
							<p class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] font-semibold text-ink/60">
								{#each g.environments as e (e.environment)}
									<span class="inline-flex items-center gap-1.5 capitalize"><span class="size-2 rounded-full {envCls(e)}"></span>{e.environment}: {envTxt(e).toLowerCase()}{#if e.environment === serverEnv}<span class="pill bg-merah/10 text-merah normal-case">server ini</span>{/if}</span>
								{/each}
							</p>
						</div>
						{@render chev()}
					</summary>
					<div class="space-y-2 border-t border-ink/10 bg-ink/[.02] p-3 sm:p-4">
						{#each g.environments as e (e.environment)}
							{@const key = keyOf(g, e)}
							{@const mine = fm?.key === key}
							<details class="overflow-hidden rounded-xl border border-ink/10 bg-surface" open={mine}>
								<summary class="flex cursor-pointer list-none flex-wrap items-center gap-2 px-4 py-3 transition hover:bg-ink/[.03] [&::-webkit-details-marker]:hidden">
									<span class="size-2.5 rounded-full {envCls(e)}"></span>
									<h4 class="text-sm font-extrabold capitalize">{e.environment}</h4>
									{#if e.environment === serverEnv}<span class="pill bg-merah/10 text-merah">{tx('pf.gw.thisServer', 'Dipakai server ini')}</span>{/if}
									<span class="pill {e.complete ? 'bg-emerald-500/15 text-emerald-700' : e.configured ? 'bg-amber-500/15 text-amber-700' : 'bg-ink/10 text-ink/60'}">{envTxt(e)}</span>
									<span class="muted ml-auto hidden text-[11px] sm:inline">v{e.version} · {fmt(e.updated_at)}</span>
									{@render chev()}
								</summary>

								<form
									method="POST"
									action="?/save"
									autocomplete="off"
									use:enhance={() => {
										busy = `save:${key}`;
										return async ({ update }) => {
											await update({ reset: true });
											busy = null;
										};
									}}
									class="space-y-3 border-t border-ink/10 p-4"
								>
									<input type="hidden" name="gateway" value={g.gateway} />
									<input type="hidden" name="environment" value={e.environment} />
									<input type="hidden" name="version" value={e.version} />
									<div class="grid gap-3 md:grid-cols-2">
										{#each e.fields as f (f.key)}
											<div class={f.multiline ? 'md:col-span-2' : ''}>
												<label class="lbl mb-1" for="{key}-{f.key}">
													{f.label}{#if f.required}<span class="text-merah"> *</span>{/if}
												</label>
												{#if f.options}
													<select id="{key}-{f.key}" name="v:{f.key}" class="inp">
														<option value="">{f.set ? `Tetap (${f.preview})` : '— tidak diubah —'}</option>
														{#each f.options as o (o)}<option value={o}>{o}</option>{/each}
													</select>
												{:else if f.multiline}
													<textarea id="{key}-{f.key}" name="v:{f.key}" rows="4" spellcheck="false" autocomplete="off" class="inp font-mono text-[11px]"
														placeholder={f.set ? 'Tersimpan. Kosongkan untuk mempertahankan, tempel PEM baru untuk mengganti.' : '-----BEGIN PRIVATE KEY-----'}></textarea>
												{:else}
													<input id="{key}-{f.key}" name="v:{f.key}" type={f.secret ? 'password' : 'text'} spellcheck="false" autocomplete="off" class="inp font-mono"
														placeholder={f.set ? (f.secret ? `Tersimpan ${f.preview} · kosongkan untuk mempertahankan` : `Saat ini: ${f.preview}`) : ''} />
												{/if}
												<p class="muted mt-1 flex flex-wrap items-center gap-x-3 text-[11px] leading-snug">
													<span class={f.set ? 'text-emerald-700' : ''}>{f.set ? '✓ terisi' : 'belum diisi'}{f.hint ? ` · ${f.hint}` : ''}</span>
													{#if f.set && !f.required}
														<label class="inline-flex items-center gap-1 font-semibold"><input type="checkbox" name="clear:{f.key}" value="1" /> hapus</label>
													{/if}
												</p>
											</div>
										{/each}
									</div>
									<div class="grid items-end gap-3 sm:grid-cols-[1fr_auto]">
										<div>
											<label class="lbl mb-1" for="{key}-note">{tx('pf.gw.fNote', 'Alasan (wajib, masuk audit)')}</label>
											<input id="{key}-note" name="note" required minlength="3" maxlength="500" class="inp" placeholder="mis. rotasi secret key" />
										</div>
										<button disabled={busy !== null} class="rounded-full bg-merah px-5 py-2.5 text-xs font-bold text-white shadow-sm transition hover:bg-merah-tua disabled:opacity-50">
											{busy === `save:${key}` ? 'Menyimpan…' : 'Simpan'}
										</button>
									</div>
									{#if mine && fm?.error}<p class="text-xs font-semibold text-red-700" role="alert">{fm.error}</p>{/if}
									{#if mine && fm?.saved}<p class="text-xs font-semibold text-emerald-700">Tersimpan. Jalankan Tes lalu Terapkan.</p>{/if}
								</form>

								{#if served(e.environment)}
									<div class="flex flex-wrap items-center gap-2 border-t border-ink/10 bg-ink/[.025] px-4 py-3">
										<span class="mr-1 text-[11px] font-bold uppercase tracking-wider text-ink/50">Langkah 2–3</span>
										{#each [{ action: 'test', label: 'Tes koneksi', cls: 'border border-sky-500/40 bg-sky-500/10 text-sky-700 hover:bg-sky-500/20' }, { action: 'apply', label: 'Terapkan ke server', cls: 'bg-ink text-surface hover:opacity-90' }] as b (b.action)}
											<form
												method="POST"
												action="?/{b.action}"
												use:enhance={() => {
													busy = `${b.action}:${key}`;
													return async ({ update }) => {
														await update({ reset: false });
														busy = null;
													};
												}}
											>
												<input type="hidden" name="gateway" value={g.gateway} />
												<input type="hidden" name="environment" value={e.environment} />
												<button disabled={busy !== null || !e.complete} title={e.complete ? '' : 'Lengkapi semua field wajib dulu'} class="rounded-full px-4 py-1.5 text-xs font-bold transition disabled:opacity-50 {b.cls}">
													{busy === `${b.action}:${key}` ? 'Memproses…' : b.label}
												</button>
											</form>
										{/each}
										{#if !e.complete}<span class="muted text-[11px]">Lengkapi field wajib dulu.</span>{/if}
									</div>
								{:else}
									<p class="muted border-t border-ink/10 px-4 py-2.5 text-[11px]">{tx('pf.gw.otherEnv', 'Tidak bisa diuji dari server ini karena server berjalan di environment lain.')}</p>
								{/if}

								{#if mine && fm?.result}
									<div class="border-t border-ink/10 bg-surface px-4 py-4" aria-live="polite">
										<ul class="space-y-2">
											{#each fm.result.steps as s (s.name)}
												<li class="flex gap-2.5 text-xs">
													<span class="grid size-5 shrink-0 place-items-center rounded-full text-[10px] font-extrabold text-white {s.ok ? 'bg-emerald-500' : 'bg-red-500'}">{s.ok ? '✓' : '✕'}</span>
													<span>
														<b>{STEP[s.name] ?? s.name}</b>
														{#if s.message}<span class="mt-0.5 block rounded-lg bg-red-500/10 px-2 py-1 text-[11px] font-semibold text-red-800">{s.message}{s.code ? ` (${s.code})` : ''}</span>{/if}
													</span>
												</li>
											{/each}
										</ul>
										<p class="mt-3 rounded-xl px-3 py-2 text-xs font-extrabold {fm.result.ok ? 'bg-emerald-500/15 text-emerald-800' : 'bg-red-500/15 text-red-800'}">
											{#if fm.result.applied}Berhasil diterapkan. Server sekarang memakai kredensial ini.
											{:else if fm.result.ok}Koneksi berhasil. Klik Terapkan untuk memakainya.
											{:else}Koneksi gagal. Kredensial yang sedang berjalan tidak diubah.{/if}
										</p>
									</div>
								{/if}
							</details>
						{/each}
					</div>
				</details>
			{/each}
		</section>
	{/if}

	{#if data.apkReleases || data.qrisgateFee}
		<section class="space-y-2" aria-labelledby="qg-list">
			<h2 id="qg-list" class="px-1 text-[11px] font-bold uppercase tracking-[.2em] text-ink/50">QRISGate</h2>

			{#if data.apkReleases}
				<details class="card overflow-hidden" open={apkMsg !== null}>
					<summary class="flex cursor-pointer list-none items-center gap-3 px-4 py-3.5 transition hover:bg-ink/[.03] sm:px-5 [&::-webkit-details-marker]:hidden">
						<div class="min-w-0 flex-1">
							<h3 class="font-display text-base font-extrabold uppercase">APK Listener</h3>
							<p class="mt-0.5 truncate text-[11px] font-semibold text-ink/60">
								{#if apkLatest}Terbaru: <b>v{apkLatest.version_name}</b> · {(apkLatest.size_bytes / 1048576).toFixed(1)} MB · {fmt(apkLatest.created_at)}{:else}Belum ada APK yang diunggah.{/if}
							</p>
						</div>
						{@render chev()}
					</summary>
					<form onsubmit={uploadApk} class="space-y-3 border-t border-ink/10 p-4 sm:p-5">
						<p class="text-xs leading-relaxed text-ink/70">
							Unggah APK listener notifikasi. Versi terbaru langsung tersedia untuk diunduh tenant di menu QRISGate. Versi yang sama tidak bisa diunggah dua kali; untuk memperbaiki, unggah versi baru. Maksimal 60 MB.
						</p>
						{#if apkLatest}<p class="break-all font-mono text-[10px] text-ink/50">sha256 {apkLatest.sha256}</p>{/if}
						<div class="grid gap-3 md:grid-cols-3">
							<div>
								<label class="lbl mb-1" for="apk-file">Berkas APK</label>
								<input id="apk-file" name="apk" type="file" accept=".apk,application/vnd.android.package-archive" required class="inp" />
							</div>
							<div>
								<label class="lbl mb-1" for="apk-ver">Versi (mis. 1.0.1)</label>
								<input id="apk-ver" name="version" required maxlength="32" class="inp" />
							</div>
							<div>
								<label class="lbl mb-1" for="apk-note">Catatan rilis (opsional)</label>
								<input id="apk-note" name="note" maxlength="500" class="inp" />
							</div>
						</div>
						<div class="flex flex-wrap items-center gap-3">
							<button disabled={busy !== null} class="rounded-full bg-merah px-5 py-2 text-xs font-bold text-white shadow-sm transition hover:bg-merah-tua disabled:opacity-50">
								{busy === 'apk' ? 'Mengunggah…' : 'Unggah APK'}
							</button>
							{#if apkMsg?.error}<p class="text-xs font-semibold text-red-700" role="alert">{apkMsg.error}</p>{/if}
							{#if apkMsg?.saved}<p class="text-xs font-semibold text-emerald-700" role="status">v{apkMsg.saved} tersimpan dan tersedia untuk diunduh.</p>{/if}
						</div>
						{#if data.apkReleases.length > 1}
							<details class="rounded-xl border border-ink/10">
								<summary class="cursor-pointer px-3 py-2 text-[11px] font-bold text-ink/60">Riwayat versi ({data.apkReleases.length - 1})</summary>
								<ul class="space-y-1 border-t border-ink/10 px-3 py-2 text-[11px] text-ink/60">
									{#each data.apkReleases.slice(1) as r (r.id)}<li>v{r.version_name} · {(r.size_bytes / 1048576).toFixed(1)} MB · {fmt(r.created_at)}</li>{/each}
								</ul>
							</details>
						{/if}
					</form>
				</details>
			{/if}

			{#if data.qrisgateFee}
				<details class="card overflow-hidden" open={rf?.key === 'qgfee'}>
					<summary class="flex cursor-pointer list-none items-center gap-3 px-4 py-3.5 transition hover:bg-ink/[.03] sm:px-5 [&::-webkit-details-marker]:hidden">
						<div class="min-w-0 flex-1">
							<h3 class="font-display text-base font-extrabold uppercase">Potongan transaksi</h3>
							<p class="mt-0.5 truncate text-[11px] font-semibold text-ink/60">Dipakai sekarang: <b>Rp{data.qrisgateFee.fee.toLocaleString('id-ID')}</b> per transaksi sukses</p>
						</div>
						{@render chev()}
					</summary>
					<form
						method="POST"
						action="?/qrisgateFee"
						autocomplete="off"
						use:enhance={() => {
							busy = 'qgfee';
							return async ({ update }) => {
								await update({ reset: false });
								busy = null;
							};
						}}
						class="space-y-3 border-t border-ink/10 p-4 sm:p-5"
					>
						<p class="text-xs leading-relaxed text-ink/70">
							Rupiah yang dipotong dari saldo EraPay tenant untuk tiap transaksi QRISGate yang sukses. Bawaan 0 (tidak memotong). Berlaku untuk transaksi berikutnya; pemotongan baru berjalan setelah integrasi QRISGate aktif.
						</p>
						<div class="grid gap-3 sm:grid-cols-2">
							<div>
								<label class="lbl mb-1" for="qgfee-fee">Potongan per transaksi (Rp)</label>
								<input id="qgfee-fee" name="fee" inputmode="numeric" pattern="[0-9]*" required value={data.qrisgateFee.fee} class="inp" />
							</div>
							<div>
								<label class="lbl mb-1" for="qgfee-reason">{tx('pf.gw.fNote', 'Alasan (wajib, masuk audit)')}</label>
								<input id="qgfee-reason" name="reason" required minlength="3" maxlength="500" class="inp" placeholder="mis. biaya layanan QRISGate" />
							</div>
						</div>
						<div class="flex flex-wrap items-center gap-3">
							<button disabled={busy !== null} class="rounded-full bg-merah px-5 py-2 text-xs font-bold text-white shadow-sm transition hover:bg-merah-tua disabled:opacity-50">
								{busy === 'qgfee' ? 'Menyimpan…' : 'Simpan potongan'}
							</button>
							{#if rf?.key === 'qgfee' && rf?.error}<p class="text-xs font-semibold text-red-700" role="alert">{rf.error}</p>{/if}
							{#if rf?.key === 'qgfee' && rf?.feeSaved}<p class="text-xs font-semibold text-emerald-700" role="status">Tersimpan.</p>{/if}
						</div>
					</form>
				</details>
			{/if}
		</section>
	{/if}

	{#if data.overview}
		<details class="card overflow-hidden">
			<summary class="flex cursor-pointer list-none items-center gap-3 px-4 py-3.5 text-sm font-extrabold transition hover:bg-ink/[.03] sm:px-5 [&::-webkit-details-marker]:hidden">
				<span class="flex-1">{tx('pf.gc.howTitle', 'Cara kerja')}: Simpan → Tes → Terapkan</span>
				{@render chev()}
			</summary>
			<ol class="list-decimal space-y-1.5 border-t border-ink/10 p-5 pl-9 text-xs leading-relaxed">
				<li><b>Simpan</b>: nilai dienkripsi dan disimpan di database. Belum mengubah apa pun yang sedang berjalan.</li>
				<li><b>Tes</b>: server mencoba meminta token ke gateway memakai kredensial tersimpan. Tidak ada uang bergerak.</li>
				<li><b>Terapkan</b>: hanya bisa jika tes lulus. Server langsung memakai kredensial baru tanpa restart; yang lama tetap dipakai kalau tes gagal.</li>
				<li>Selama belum ada kredensial tersimpan, server memakai <code class="font-mono">DOKU_*</code> dari .env seperti sebelumnya. Server berjalan di <b class="capitalize">{serverEnv}</b>; hanya environment itu yang bisa diuji dan diterapkan.</li>
			</ol>
		</details>
	{/if}
</div>
