// Teks landing page (/). Terpisah dari kamus panel supaya berkas messages.ts tidak membengkak.
// Hanya teks yang diterjemahkan; ikon, gambar, dan kelas CSS tetap di komponen.
import type { Lang } from './messages';

type Baris = [string, string?];
type Tp = [string, string]; // [judul, deskripsi]

export interface Lp {
	meta: { title: string; desc: string };
	hdr: { nav: [string, string, string]; masuk: string; daftar: string; menu: string; navAria: string; navSel: string };
	hero: {
		badge: string;
		h1: string;
		frasa: [string, string][];
		sub: string;
		mulai: string;
		fitur: string;
		stat: [string, string][];
		kalk: { judul: string; ket: string; nominal: string; geser: string; mdr: string; jasa: string; diterima: string; berhasil: string };
	};
	strip: string[];
	kenali: {
		unggulan: Tp[];
		alt: string;
		tulisan: string;
		judul: string;
		satuQr: [string, string];
		pengantar: string;
		kuas: [string, string];
		pahami: [string, string];
		pahamiSub: string;
		gelembung: [string, string];
		tabsAria: string;
		pertanyaan: string; // "{n}" dan "{total}"
		tahu: string;
		ulangi: string;
		lanjut: string; // "{k}"
		prev: string;
		cta: string;
		items: { k: string; tanya: string; isi: string; fakta: [string, string][]; kartu: Tp[]; langkah: string[]; tahu: string }[];
	};
	tentang: {
		tag: string;
		judul: [string, string, string];
		sub: string;
		ledger: { tag: string; judul: string; isi: string; jurnal: string; ilustrasi: string; akun: string; debit: string; kredit: string; total: string; akunJurnal: [string, string, string]; mdr: string };
		aman: { tag: string; judul: string; daftar: Tp[] };
		idem: { tag: string; judul: string; isi: string; retry: string; sama: string };
		state: { tag: string; judul: string; isi1: [string, string]; isi2: string; ket: string };
		adapter: { tag: string; judul: string; aria: string; lapis: [string, string, string, string]; isi: string };
		stack: { label: string; daftar: string[] };
	};
	cara: {
		tag: string;
		judul: string;
		sub: string;
		daftar: string;
		nodeAria: string;
		tabAria: string;
		node: string[];
		contoh: string;
		langkah: { t: string; d: string; tag: string[]; kode: Baris[] }[];
	};
	fitur: {
		tag: string;
		judul: string;
		sub: string;
		ringkas: [string, string][];
		daftar: { t: string; d: string; poin: [string, string] }[];
	};
	cta: { judul: string; isi: string; daftar: string; masuk: string };
}

const id: Lp = {
	meta: {
		title: 'EraPay — Terima QRIS, dana tercatat rapi',
		desc: 'EraPay: platform pembayaran QRIS untuk merchant dengan ledger double-entry, rekonsiliasi settlement, payout maker-checker, dan satu API untuk POS.'
	},
	hdr: { nav: ['Kenali QRIS', 'Fitur', 'Cara Kerja'], masuk: 'Masuk', daftar: 'Daftar Merchant', menu: 'Buka menu', navAria: 'Navigasi utama', navSel: 'Navigasi seluler' },
	hero: {
		badge: 'Saatnya Merdeka Bertransaksi · Sandbox tersedia',
		h1: 'Terima QRIS',
		frasa: [['Dana Tercatat Rapi', 'dan Transparan'], ['Transaksi Otomatis', '24/7'], ['Bebas Rekap', 'Manual'], ['Laporan Keuangan', 'Real-Time'], ['Integrasi Instan', 'dan Mudah']],
		sub: 'EraPay menyatukan pembayaran QRIS, ledger, rekonsiliasi settlement, dan payout dalam satu platform. Satu API untuk POS, satu dashboard untuk seluruh tim.',
		mulai: 'Mulai Sekarang',
		fitur: 'Lihat Fitur',
		stat: [['Biaya Jasa UMKM', '0,3%'], ['Keamanan', 'Berlapis'], ['Payout', 'Maker-checker']],
		kalk: {
			judul: 'Berapa yang Anda terima?',
			ket: 'Simulasi tarif UMKM dan Perseorangan secara transparan.',
			nominal: 'Nominal transaksi',
			geser: 'Geser nominal transaksi',
			mdr: 'MDR QRIS (0,7%)',
			jasa: 'Biaya Jasa EraPay (0,3%)',
			diterima: 'Diterima merchant',
			berhasil: 'Pembayaran berhasil'
		}
	},
	strip: ['QRIS untuk merchant', 'Ledger double-entry', 'Payout maker-checker', 'Rekonsiliasi settlement', 'Webhook bertanda tangan', 'Refund & dispute', 'API + Idempotency-Key', 'Audit log lengkap'],
	kenali: {
		unggulan: [['Aman', 'Terpercaya'], ['Cepat', 'Mudah'], ['Untuk Semua', 'Kalangan']],
		alt: 'Ilustrasi pembeli membayar dengan QRIS kepada pedagang sayur di pasar tradisional, dengan Monas dan bendera Merah Putih di latar belakang',
		tulisan: 'Kenali, Pahami,',
		judul: 'Gunakan QRIS',
		satuQr: ['Satu QR, untuk', 'semua transaksi.'],
		pengantar: 'QRIS (Quick Response Code Indonesian Standard) adalah standar kode QR nasional untuk memudahkan pembayaran digital di seluruh Indonesia.',
		kuas: ['Dari Pasar Tradisional', 'hingga Modern, Semua Bisa!'],
		pahami: ['Pahami QRIS', 'dengan'],
		pahamiSub: 'Kenali lebih dalam tentang QRIS melalui 6 pertanyaan penting berikut. Klik setiap pertanyaan untuk melihat penjelasannya.',
		gelembung: ['Hmm… QRIS itu', 'apa sih?'],
		tabsAria: 'Pertanyaan 5W + 1H tentang QRIS',
		pertanyaan: 'Pertanyaan {n} dari {total}',
		tahu: 'Tahukah kamu?',
		ulangi: 'Ulangi dari awal',
		lanjut: 'Lanjut: {k}',
		prev: 'Pertanyaan sebelumnya',
		cta: 'Terima QRIS untuk usaha Anda dengan EraPay',
		items: [
			{
				k: 'What',
				tanya: 'Apa itu QRIS?',
				isi: 'QRIS (Quick Response Code Indonesian Standard) adalah standar kode QR nasional yang ditetapkan Bank Indonesia bersama Asosiasi Sistem Pembayaran Indonesia (ASPI). Satu kode QRIS bisa dibayar dari aplikasi m-banking maupun dompet digital mana pun.',
				fakta: [['Berlaku sejak', '1 Januari 2020'], ['Pengatur', 'Bank Indonesia'], ['Dasar standar', 'EMVCo internasional']],
				kartu: [],
				langkah: [],
				tahu: 'Dulu setiap aplikasi punya QR sendiri, sampai kasir harus memajang banyak stiker QR. QRIS menyatukannya menjadi satu.'
			},
			{
				k: 'Who',
				tanya: 'Siapa yang menggunakannya?',
				isi: 'Semua orang! Pembeli cukup punya aplikasi pembayaran yang mendukung QRIS, sedangkan penjualnya bisa siapa saja.',
				fakta: [],
				kartu: [['Pembeli', 'Siapa pun yang punya m-banking atau dompet digital.'], ['Pedagang & UMKM', 'Lapak pasar, warung, kafe, hingga toko modern.'], ['Usaha online', 'Toko daring, tiket, dan tagihan digital.'], ['Lembaga sosial', 'Donasi di rumah ibadah dan yayasan.']],
				langkah: [],
				tahu: 'Pedagang tidak perlu mesin EDC. Cukup stiker QRIS dan ponsel untuk mengecek notifikasi pembayaran.'
			},
			{
				k: 'When',
				tanya: 'Kapan bisa digunakan?',
				isi: 'Kapan saja, 24 jam sehari dan 7 hari seminggu, termasuk hari libur. Pembayaran diproses seketika sehingga pembeli dan penjual langsung tahu transaksi berhasil.',
				fakta: [['Jam layanan', '24/7'], ['Konfirmasi', 'Real-time']],
				kartu: [['Pagi', 'Belanja sayur di pasar.'], ['Siang', 'Makan siang di warung.'], ['Malam', 'Nongkrong di kafe.']],
				langkah: [],
				tahu: 'Pembayaran QRIS langsung berstatus berhasil, tapi dana ke rekening pedagang mengikuti jadwal settlement penyelenggara.'
			},
			{
				k: 'Where',
				tanya: 'Di mana bisa digunakan?',
				isi: 'Di setiap merchant yang memajang tanda QRIS di seluruh Indonesia, dari Sabang sampai Merauke. Bisa di toko fisik maupun saat checkout online.',
				fakta: [],
				kartu: [['Toko fisik', 'Scan stiker QRIS atau QR di layar kasir.'], ['Online', 'QRIS tampil di halaman pembayaran.'], ['Luar negeri', 'Di negara mitra seperti Thailand, Malaysia, dan Singapura.']],
				langkah: [],
				tahu: 'Dengan QRIS antarnegara, kamu bisa bayar di negara mitra pakai aplikasi dari Indonesia, tanpa tukar uang tunai.'
			},
			{
				k: 'Why',
				tanya: 'Mengapa menggunakan QRIS?',
				isi: 'Praktis untuk pembeli, rapi untuk penjual.',
				fakta: [],
				kartu: [['Pembeli', 'Tak perlu bawa uang tunai, cukup satu aplikasi apa pun, dan ada bukti transaksi.'], ['Penjual', 'Tak perlu uang kembalian, bebas uang palsu, dan transaksi tercatat otomatis.']],
				langkah: [],
				tahu: 'Catatan transaksi QRIS bisa jadi riwayat usaha yang membantu UMKM saat mengajukan pembiayaan.'
			},
			{
				k: 'How',
				tanya: 'Bagaimana cara membayar?',
				isi: 'Cukup empat langkah, kurang dari satu menit:',
				fakta: [],
				kartu: [],
				langkah: ['Buka aplikasi m-banking atau dompet digital', 'Pindai kode QRIS di kasir', 'Cek nama merchant dan nominal', 'Masukkan PIN, pembayaran selesai!'],
				tahu: 'Selalu cek nama merchant sebelum memasukkan PIN, dan jangan pernah membagikan PIN kepada siapa pun.'
			}
		]
	},
	tentang: {
		tag: 'Tentang EraPay',
		judul: ['Infrastruktur pembayaran yang dibangun seperti ', 'sistem keuangan', '.'],
		sub: 'Bukan sekadar QR. Setiap rupiah melewati validasi server, dicatat double-entry, dan bisa ditelusuri dari transaksi hingga laporan settlement.',
		ledger: {
			tag: 'Ledger',
			judul: 'Double-entry & append-only',
			isi: 'Uang disimpan sebagai integer, tidak pernah float. Saldo diturunkan dari jurnal yang tidak bisa diubah; koreksi lewat reversal, bukan hapus.',
			jurnal: 'journal · payment.succeeded · gross',
			ilustrasi: '(ilustrasi)',
			akun: 'Akun',
			debit: 'Debit',
			kredit: 'Kredit',
			total: 'Total',
			akunJurnal: ['Kliring gateway', 'Kewajiban ke merchant', 'Pendapatan Biaya Jasa'],
			mdr: 'dipotong gateway sebelum masuk kliring · tarif final mengikuti kontrak'
		},
		aman: {
			tag: 'Security by design',
			judul: 'Berlapis, bukan tempelan',
			daftar: [['AES-256-GCM', 'Kredensial & dokumen terenkripsi at rest'], ['HMAC-SHA256', 'Webhook bertanda tangan + anti-replay'], ['Tenant isolation', 'merchant_id dari identitas, bukan input'], ['MFA + maker-checker', 'Payout butuh dua orang berbeda'], ['RBAC', 'Hak akses minimum per peran'], ['Audit log', 'Setiap aksi sensitif tercatat']]
		},
		idem: { tag: 'Idempotency', judul: 'Request ganda? Tetap satu transaksi.', isi: 'Dijaga unique constraint PostgreSQL, bukan sekadar lock di cache.', retry: 'retry', sama: '(sama)' },
		state: {
			tag: 'State machine',
			judul: 'Status final hanya dari sumber resmi',
			isi1: ['Berhasil hanya dari webhook gateway yang lolos verifikasi atau inquiry resmi. Timeout jadi ', ' lalu dipastikan, bukan ditebak gagal.'],
			isi2: '',
			ket: 'Pembayaran, settlement, dan refund punya state machine terpisah.'
		},
		adapter: {
			tag: 'Gateway-agnostic',
			judul: 'Satu API, adapter di belakang',
			aria: 'POS ke EraPay API ke adapter ke gateway',
			lapis: ['POS / aplikasi Anda', 'EraPay Public API', 'Adapter gateway', 'Penyedia pembayaran'],
			isi: 'Istilah dan format gateway berhenti di adapter. Kode POS Anda tidak ikut berubah.'
		},
		stack: { label: 'Dibangun dengan', daftar: ['Go modular monolith', 'PostgreSQL · ACID', 'Redis', 'Outbox worker', 'REST + JSON', 'TLS everywhere'] }
	},
	cara: {
		tag: 'Cara kerja',
		judul: 'Dari scan QRIS hingga dana di rekening, tiap langkah bisa diaudit',
		sub: 'Lima tahap yang dijalankan server EraPay untuk setiap pembayaran. Pilih tahap untuk melihat apa yang terjadi di baliknya.',
		daftar: 'Daftar Merchant',
		nodeAria: 'Komponen yang terlibat',
		tabAria: 'Tahap pembayaran',
		node: ['POS Anda', 'EraPay API', 'Gateway QRIS', 'Ledger', 'Rekening merchant'],
		contoh: 'Contoh ilustrasi · nilai & ID disamarkan',
		langkah: [
			{
				t: 'Buat pembayaran',
				d: 'POS memanggil satu endpoint. Fee dihitung server, QR dinamis dibuat sesuai nominal.',
				tag: ['REST', 'Idempotency-Key', 'Bearer API key'],
				kode: [['→ POST /v1/payments', 'k'], ['  Authorization: Bearer sk_••••••••', 'm'], ['  Idempotency-Key: 7c1f…e92a', 'm'], ['  { "merchant_reference": "INV-1042", "amount": 25000,'], ['    "currency": "IDR", "payment_method": "QRIS" }'], [''], ['← 201 Created', 'ok'], ['  { "id": "pay_8Hq2…", "status": "pending",'], ['    "fee": { "mdr_fee": 175, "platform_fee": 75,', 'e'], ['             "merchant_net": 24750 },', 'e'], ['    "qr_code": { "type": "provider_defined", … } }']]
			},
			{
				t: 'Pelanggan scan & bayar',
				d: 'Pelanggan memindai QRIS dari aplikasi apa pun dan membayar tepat sebesar nominal transaksi.',
				tag: ['QRIS dinamis', 'Nominal terkunci'],
				kode: [['10:21:04  QR tampil di layar kasir', 'm'], ['10:21:04  status = pending   amount = 25000 IDR'], ['10:21:19  pelanggan memindai QR dari aplikasinya', 'm'], ['10:21:23  pembayaran diproses penyedia QRIS', 'm'], ['10:21:23  menunggu notifikasi resmi dari gateway…', 'w'], [''], ['# layar "berhasil" di ponsel pelanggan belum dianggap final', 'c'], ['# status final hanya dari server, lihat langkah berikutnya', 'c']]
			},
			{
				t: 'Verifikasi status final',
				d: 'Notifikasi gateway diverifikasi tanda tangan, nominal, dan duplikasinya sebelum status berubah.',
				tag: ['Signature check', 'Dedup', 'Inquiry fallback'],
				kode: [['→ POST /webhooks/{gateway}', 'k'], ['  ✓ tanda tangan valid', 'ok'], ['  ✓ amount 25000 = 25000', 'ok'], ['  ✓ event belum pernah diproses (dedup)', 'ok'], ['  pay_8Hq2…  pending → succeeded', 'e'], [''], ['# timeout / respons ambigu?', 'c'], ['  status → unknown → inquiry resmi ke gateway', 'w'], ['  # tidak pernah ditebak "gagal"', 'c']]
			},
			{
				t: 'Ledger & notifikasi',
				d: 'Status, jurnal double-entry, dan event webhook ditulis dalam satu transaksi database (outbox).',
				tag: ['ACID', 'Outbox', 'HMAC-SHA256', 'Retry ×8'],
				kode: [['BEGIN;', 'k'], ['  journal  Dr Kliring gateway        24.825', 'e'], ['           Cr Kewajiban ke merchant  24.750', 'e'], ['           Cr Pendapatan Biaya Jasa      75', 'e'], ['  event    payment.succeeded  (outbox)'], ['COMMIT;   Σ debit = Σ kredit ✓', 'ok'], [''], ['→ POST https://pos-anda.example/webhook', 'k'], ['  X-EraPay-Signature: t=1760600483,v1=5f2a…', 'm'], ['← 200 OK   attempt 1/8', 'ok']]
			},
			{
				t: 'Rekonsiliasi & payout',
				d: 'Laporan settlement dicocokkan dengan ledger, lalu payout dibuat dan disetujui dua orang berbeda.',
				tag: ['Rekonsiliasi', 'Maker-checker', 'MFA'],
				kode: [['reconcile laporan-settlement.csv', 'k'], ['  pay_8Hq2…  gross 25.000  ✓ matched', 'ok'], ['  saldo merchant: pending → available', 'e'], [''], ['payout po_31xK…  amount = saldo tersedia', 'k'], ['  dibuat     : Finance   (maker)', 'm'], ['  disetujui  : Owner     (checker, MFA ✓)', 'ok'], ['  rekening   : ••••4821  (allowlist ✓)', 'm'], ['  status     : dikirim ke rekening merchant', 'e']]
			}
		]
	},
	fitur: {
		tag: 'Ikhtisar fitur utama',
		judul: 'Solusi lengkap untuk pembayaran modern',
		sub: 'Urusan pembayaran beres, Anda bisa fokus melayani pelanggan. Semua kebutuhan dari terima QRIS sampai dana masuk rekening ada di satu platform.',
		ringkas: [['6', 'modul terintegrasi'], ['1', 'API untuk POS'], ['3', 'status terpisah']],
		daftar: [
			{ t: 'Pembayaran QRIS', d: 'Buat QR dinamis, pantau status real-time, dan terima notifikasi ke sistem Anda.', poin: ['QR sesuai nominal dengan masa berlaku', 'Webhook bertanda tangan + retry otomatis'] },
			{ t: 'Dashboard Transaksi', d: 'Cari dan filter setiap transaksi. Status pembayaran, settlement, dan refund tampil terpisah.', poin: ['Filter status, tanggal, dan outlet', 'Timeline lengkap per pembayaran'] },
			{ t: 'Settlement & Rekonsiliasi', d: 'Cocokkan laporan gateway dengan ledger, tandai exception, dan telusuri selisih hingga sumbernya.', poin: ['Impor laporan settlement', 'Selisih tercatat dan bisa ditelusuri'] },
			{ t: 'Payout & Saldo Merchant', d: 'Saldo dihitung dari ledger, payout idempoten, dan setiap pencairan butuh persetujuan.', poin: ['Saldo pending, tersedia, dan ditahan', 'Maker-checker + MFA saat approve'] },
			{ t: 'Refund, Cancel & Dispute', d: 'Alur pengembalian dana dengan izin khusus dan persetujuan berlapis.', poin: ['Hanya peran berwenang', 'Tercatat di ledger dan audit log'] },
			{ t: 'Merchant, Outlet & Peran', d: 'Onboarding KYC, banyak outlet, serta API key dan webhook per merchant.', poin: ['Owner, kasir, finance, dan ops', 'Hak akses sesuai peran'] }
		]
	},
	cta: { judul: 'Beli Kopi 5 Detik, Nunggu QRIS-nya 5 Menit... Pelanggan Keburu Emosi!', isi: 'Muluskan transaksi dari sekarang! Daftar, lengkapi data, uji coba sistem, lalu siap Go Live tanpa drama.', daftar: 'Daftar Merchant', masuk: 'Masuk ke Dashboard' }
};

const en: Lp = {
	meta: {
		title: 'EraPay — Accept QRIS, funds recorded neatly',
		desc: 'EraPay: a QRIS payment platform for merchants with a double-entry ledger, settlement reconciliation, maker-checker payouts, and a single API for your POS.'
	},
	hdr: { nav: ['Discover QRIS', 'Features', 'How It Works'], masuk: 'Log in', daftar: 'Become a Merchant', menu: 'Open menu', navAria: 'Main navigation', navSel: 'Mobile navigation' },
	hero: {
		badge: 'Time to be free to transact · Sandbox available',
		h1: 'Accept QRIS',
		frasa: [['Funds Recorded Neatly', '& Transparently'], ['Automated Transactions', '24/7'], ['No More Manual', 'Reconciliation'], ['Real-Time', 'Financial Reports'], ['Instant & Easy', 'Integration']],
		sub: 'EraPay brings QRIS payments, ledger, settlement reconciliation, and payouts into one platform. One API for your POS, one dashboard for your whole team.',
		mulai: 'Get Started',
		fitur: 'See Features',
		stat: [['SME service fee', '0.3%'], ['Security', 'Layered'], ['Payout', 'Maker-checker']],
		kalk: {
			judul: 'How much will you receive?',
			ket: 'A transparent fee simulation for SMEs and individuals.',
			nominal: 'Transaction amount',
			geser: 'Slide to set the transaction amount',
			mdr: 'QRIS MDR (0.7%)',
			jasa: 'EraPay Service Fee (0.3%)',
			diterima: 'Received by merchant',
			berhasil: 'Payment successful'
		}
	},
	strip: ['QRIS for merchants', 'Double-entry ledger', 'Maker-checker payouts', 'Settlement reconciliation', 'Signed webhooks', 'Refunds & disputes', 'API + Idempotency-Key', 'Complete audit log'],
	kenali: {
		unggulan: [['Secure', '& Trusted'], ['Fast', '& Easy'], ['For Everyone', 'Everywhere']],
		alt: 'Illustration of a customer paying a vegetable seller with QRIS in a traditional market, with the Monas and the Red-and-White flag in the background',
		tulisan: 'Discover, Understand,',
		judul: 'Use QRIS',
		satuQr: ['One QR, for', 'every transaction.'],
		pengantar: 'QRIS (Quick Response Code Indonesian Standard) is the national QR code standard that makes digital payments easy across Indonesia.',
		kuas: ['From Traditional Markets', 'to Modern Stores, Anyone Can!'],
		pahami: ['Understand QRIS', 'with'],
		pahamiSub: 'Learn more about QRIS through these 6 key questions. Click each question to see the answer.',
		gelembung: ['Hmm… what is', 'QRIS anyway?'],
		tabsAria: '5W + 1H questions about QRIS',
		pertanyaan: 'Question {n} of {total}',
		tahu: 'Did you know?',
		ulangi: 'Start over',
		lanjut: 'Next: {k}',
		prev: 'Previous question',
		cta: 'Accept QRIS for your business with EraPay',
		items: [
			{
				k: 'What',
				tanya: 'What is QRIS?',
				isi: 'QRIS (Quick Response Code Indonesian Standard) is the national QR code standard set by Bank Indonesia together with the Indonesian Payment System Association (ASPI). A single QRIS code can be paid from any m-banking app or digital wallet.',
				fakta: [['Effective since', 'January 1, 2020'], ['Regulator', 'Bank Indonesia'], ['Based on', 'International EMVCo']],
				kartu: [],
				langkah: [],
				tahu: 'Each app used to have its own QR, so cashiers had to display a pile of QR stickers. QRIS merges them into one.'
			},
			{
				k: 'Who',
				tanya: 'Who uses it?',
				isi: 'Everyone! Buyers only need a payment app that supports QRIS, and sellers can be anyone.',
				fakta: [],
				kartu: [['Buyers', 'Anyone with an m-banking app or digital wallet.'], ['Merchants & SMEs', 'Market stalls, food stalls, cafés, up to modern stores.'], ['Online businesses', 'Online shops, tickets, and digital bills.'], ['Social institutions', 'Donations at places of worship and foundations.']],
				langkah: [],
				tahu: 'Merchants don’t need an EDC machine. A QRIS sticker and a phone to check payment notifications are enough.'
			},
			{
				k: 'When',
				tanya: 'When can it be used?',
				isi: 'Anytime, 24 hours a day and 7 days a week, including holidays. Payments are processed instantly so buyer and seller know right away that the transaction succeeded.',
				fakta: [['Service hours', '24/7'], ['Confirmation', 'Real-time']],
				kartu: [['Morning', 'Buying vegetables at the market.'], ['Afternoon', 'Lunch at a local eatery.'], ['Night', 'Hanging out at a café.']],
				langkah: [],
				tahu: 'A QRIS payment is marked successful immediately, but funds reach the merchant’s account on the provider’s settlement schedule.'
			},
			{
				k: 'Where',
				tanya: 'Where can it be used?',
				isi: 'At every merchant displaying the QRIS sign across Indonesia, from Sabang to Merauke. In physical stores and at online checkout.',
				fakta: [],
				kartu: [['Physical stores', 'Scan the QRIS sticker or the QR on the cashier screen.'], ['Online', 'QRIS shows up on the payment page.'], ['Overseas', 'In partner countries such as Thailand, Malaysia, and Singapore.']],
				langkah: [],
				tahu: 'With cross-border QRIS, you can pay in partner countries using an Indonesian app, without exchanging cash.'
			},
			{
				k: 'Why',
				tanya: 'Why use QRIS?',
				isi: 'Convenient for buyers, tidy for sellers.',
				fakta: [],
				kartu: [['Buyers', 'No need to carry cash, just any one app, and you get proof of payment.'], ['Sellers', 'No change to hand back, no counterfeit bills, and transactions are recorded automatically.']],
				langkah: [],
				tahu: 'QRIS transaction records can become a business history that helps SMEs when applying for financing.'
			},
			{
				k: 'How',
				tanya: 'How do you pay?',
				isi: 'Just four steps, under a minute:',
				fakta: [],
				kartu: [],
				langkah: ['Open your m-banking or digital wallet app', 'Scan the QRIS code at the cashier', 'Check the merchant name and amount', 'Enter your PIN, payment complete!'],
				tahu: 'Always check the merchant name before entering your PIN, and never share your PIN with anyone.'
			}
		]
	},
	tentang: {
		tag: 'About EraPay',
		judul: ['Payment infrastructure built like a ', 'financial system', '.'],
		sub: 'More than just a QR. Every rupiah goes through server validation, is recorded double-entry, and can be traced from the transaction to the settlement report.',
		ledger: {
			tag: 'Ledger',
			judul: 'Double-entry & append-only',
			isi: 'Money is stored as integers, never floats. Balances are derived from an immutable journal; corrections happen through reversals, not deletions.',
			jurnal: 'journal · payment.succeeded · gross',
			ilustrasi: '(illustration)',
			akun: 'Account',
			debit: 'Debit',
			kredit: 'Credit',
			total: 'Total',
			akunJurnal: ['Gateway clearing', 'Merchant liability', 'Service fee revenue'],
			mdr: 'deducted by the gateway before clearing · final rates follow your contract'
		},
		aman: {
			tag: 'Security by design',
			judul: 'Layered, not bolted on',
			daftar: [['AES-256-GCM', 'Credentials & documents encrypted at rest'], ['HMAC-SHA256', 'Signed webhooks + anti-replay'], ['Tenant isolation', 'merchant_id from identity, not input'], ['MFA + maker-checker', 'Payouts need two different people'], ['RBAC', 'Least-privilege access per role'], ['Audit log', 'Every sensitive action is recorded']]
		},
		idem: { tag: 'Idempotency', judul: 'Duplicate request? Still one transaction.', isi: 'Enforced by a PostgreSQL unique constraint, not just a cache lock.', retry: 'retry', sama: '(same)' },
		state: {
			tag: 'State machine',
			judul: 'Final status only from official sources',
			isi1: ['Success comes only from a verified gateway webhook or an official inquiry. A timeout becomes ', ', then gets confirmed, never guessed as failed.'],
			isi2: '',
			ket: 'Payments, settlements, and refunds each have their own state machine.'
		},
		adapter: {
			tag: 'Gateway-agnostic',
			judul: 'One API, adapters behind it',
			aria: 'POS to EraPay API to adapter to gateway',
			lapis: ['Your POS / app', 'EraPay Public API', 'Gateway adapter', 'Payment provider'],
			isi: 'Gateway terms and formats stop at the adapter. Your POS code doesn’t have to change.'
		},
		stack: { label: 'Built with', daftar: ['Go modular monolith', 'PostgreSQL · ACID', 'Redis', 'Outbox worker', 'REST + JSON', 'TLS everywhere'] }
	},
	cara: {
		tag: 'How it works',
		judul: 'From QRIS scan to funds in your account, every step is auditable',
		sub: 'Five stages the EraPay server runs for every payment. Pick a stage to see what happens behind it.',
		daftar: 'Become a Merchant',
		nodeAria: 'Components involved',
		tabAria: 'Payment stages',
		node: ['Your POS', 'EraPay API', 'QRIS Gateway', 'Ledger', 'Merchant account'],
		contoh: 'Illustrative example · values & IDs are masked',
		langkah: [
			{
				t: 'Create a payment',
				d: 'The POS calls a single endpoint. The server calculates fees and generates a dynamic QR for the exact amount.',
				tag: ['REST', 'Idempotency-Key', 'Bearer API key'],
				kode: [['→ POST /v1/payments', 'k'], ['  Authorization: Bearer sk_••••••••', 'm'], ['  Idempotency-Key: 7c1f…e92a', 'm'], ['  { "merchant_reference": "INV-1042", "amount": 25000,'], ['    "currency": "IDR", "payment_method": "QRIS" }'], [''], ['← 201 Created', 'ok'], ['  { "id": "pay_8Hq2…", "status": "pending",'], ['    "fee": { "mdr_fee": 175, "platform_fee": 75,', 'e'], ['             "merchant_net": 24750 },', 'e'], ['    "qr_code": { "type": "provider_defined", … } }']]
			},
			{
				t: 'Customer scans & pays',
				d: 'The customer scans the QRIS from any app and pays exactly the transaction amount.',
				tag: ['Dynamic QRIS', 'Locked amount'],
				kode: [['10:21:04  QR shown on the cashier screen', 'm'], ['10:21:04  status = pending   amount = 25000 IDR'], ['10:21:19  customer scans the QR from their app', 'm'], ['10:21:23  payment processed by the QRIS provider', 'm'], ['10:21:23  waiting for official gateway notification…', 'w'], [''], ['# the "success" screen on the customer’s phone is not final', 'c'], ['# final status comes only from the server, see next step', 'c']]
			},
			{
				t: 'Verify final status',
				d: 'The gateway notification is checked for signature, amount, and duplicates before the status changes.',
				tag: ['Signature check', 'Dedup', 'Inquiry fallback'],
				kode: [['→ POST /webhooks/{gateway}', 'k'], ['  ✓ signature valid', 'ok'], ['  ✓ amount 25000 = 25000', 'ok'], ['  ✓ event not processed before (dedup)', 'ok'], ['  pay_8Hq2…  pending → succeeded', 'e'], [''], ['# timeout / ambiguous response?', 'c'], ['  status → unknown → official inquiry to gateway', 'w'], ['  # never guessed as "failed"', 'c']]
			},
			{
				t: 'Ledger & notification',
				d: 'The status, double-entry journal, and webhook event are written in a single database transaction (outbox).',
				tag: ['ACID', 'Outbox', 'HMAC-SHA256', 'Retry ×8'],
				kode: [['BEGIN;', 'k'], ['  journal  Dr Gateway clearing       24.825', 'e'], ['           Cr Merchant liability     24.750', 'e'], ['           Cr Service fee revenue        75', 'e'], ['  event    payment.succeeded  (outbox)'], ['COMMIT;   Σ debit = Σ credit ✓', 'ok'], [''], ['→ POST https://your-pos.example/webhook', 'k'], ['  X-EraPay-Signature: t=1760600483,v1=5f2a…', 'm'], ['← 200 OK   attempt 1/8', 'ok']]
			},
			{
				t: 'Reconciliation & payout',
				d: 'The settlement report is matched against the ledger, then a payout is created and approved by two different people.',
				tag: ['Reconciliation', 'Maker-checker', 'MFA'],
				kode: [['reconcile settlement-report.csv', 'k'], ['  pay_8Hq2…  gross 25.000  ✓ matched', 'ok'], ['  merchant balance: pending → available', 'e'], [''], ['payout po_31xK…  amount = available balance', 'k'], ['  created    : Finance   (maker)', 'm'], ['  approved   : Owner     (checker, MFA ✓)', 'ok'], ['  account    : ••••4821  (allowlist ✓)', 'm'], ['  status     : sent to merchant account', 'e']]
			}
		]
	},
	fitur: {
		tag: 'Key features at a glance',
		judul: 'A complete solution for modern payments',
		sub: 'Payments sorted, so you can focus on serving customers. Everything from accepting QRIS to funds landing in your account lives on one platform.',
		ringkas: [['6', 'integrated modules'], ['1', 'API for your POS'], ['3', 'separate statuses']],
		daftar: [
			{ t: 'QRIS Payments', d: 'Create dynamic QR codes, track status in real time, and get notified in your own system.', poin: ['QR matched to the amount, with an expiry', 'Signed webhooks + automatic retries'] },
			{ t: 'Transaction Dashboard', d: 'Search and filter every transaction. Payment, settlement, and refund statuses are shown separately.', poin: ['Filter by status, date, and outlet', 'Full timeline for each payment'] },
			{ t: 'Settlement & Reconciliation', d: 'Match gateway reports against the ledger, flag exceptions, and trace differences to their source.', poin: ['Import settlement reports', 'Differences are recorded and traceable'] },
			{ t: 'Payouts & Merchant Balance', d: 'Balances are computed from the ledger, payouts are idempotent, and every disbursement needs approval.', poin: ['Pending, available, and held balances', 'Maker-checker + MFA on approval'] },
			{ t: 'Refunds, Cancels & Disputes', d: 'A refund flow with dedicated permissions and layered approvals.', poin: ['Authorized roles only', 'Recorded in the ledger and audit log'] },
			{ t: 'Merchants, Outlets & Roles', d: 'KYC onboarding, multiple outlets, plus API keys and webhooks per merchant.', poin: ['Owner, cashier, finance, and ops', 'Access rights by role'] }
		]
	},
	cta: { judul: 'Coffee Takes 5 Seconds, Waiting for QRIS Takes 5 Minutes... Customers Are Losing It!', isi: 'Smooth out your transactions starting today! Register, complete your data, test the system, then Go Live drama-free.', daftar: 'Become a Merchant', masuk: 'Go to Dashboard' }
};

const ja: Lp = {
	meta: {
		title: 'EraPay — QRISを受け付け、資金をきれいに記録',
		desc: 'EraPay：複式簿記の元帳、精算の照合、メーカー・チェッカー方式の出金、POS向け単一APIを備えた、加盟店向けQRIS決済プラットフォーム。'
	},
	hdr: { nav: ['QRISを知る', '機能', '仕組み'], masuk: 'ログイン', daftar: '加盟店登録', menu: 'メニューを開く', navAria: 'メインナビゲーション', navSel: 'モバイルナビゲーション' },
	hero: {
		badge: '自由に取引できる時代へ · サンドボックス提供中',
		h1: 'QRISを導入',
		frasa: [['資金を正確に記録', '透明性も確保'], ['取引を自動化', '24時間365日'], ['手作業の照合は', 'もう不要'], ['リアルタイムの', '財務レポート'], ['すぐに使える', 'かんたん連携']],
		sub: 'EraPayは、QRIS決済・元帳・精算の照合・出金をひとつのプラットフォームに統合。POS向けのAPIはひとつ、チーム全員が使うダッシュボードもひとつです。',
		mulai: '今すぐ始める',
		fitur: '機能を見る',
		stat: [['中小事業者の手数料', '0.3%'], ['セキュリティ', '多層防御'], ['出金', 'メーカー・チェッカー']],
		kalk: {
			judul: '受取額はいくら？',
			ket: '中小事業者・個人向け手数料の透明なシミュレーション。',
			nominal: '取引金額',
			geser: 'スライドして取引金額を設定',
			mdr: 'QRIS MDR（0.7%）',
			jasa: 'EraPayサービス手数料（0.3%）',
			diterima: '加盟店の受取額',
			berhasil: '支払いが完了しました'
		}
	},
	strip: ['加盟店向けQRIS', '複式簿記の元帳', 'メーカー・チェッカー出金', '精算の照合', '署名付きWebhook', '返金・紛争対応', 'API + Idempotency-Key', '完全な監査ログ'],
	kenali: {
		unggulan: [['安全', '安心'], ['速くて', '簡単'], ['すべての', '人のために']],
		alt: '伝統市場で野菜売りにQRISで支払う買い物客のイラスト。背景にはモナスと紅白の国旗',
		tulisan: '知って、理解して、',
		judul: 'QRISを使おう',
		satuQr: ['ひとつのQRで、', 'すべての取引を。'],
		pengantar: 'QRIS（Quick Response Code Indonesian Standard）は、インドネシア全土でデジタル決済をかんたんにする国家統一のQRコード規格です。',
		kuas: ['伝統市場から', '最新の店舗まで、だれでも！'],
		pahami: ['QRISを理解する', '5W1Hで'],
		pahamiSub: '6つの大切な質問でQRISをもっと深く知りましょう。質問をクリックすると答えが表示されます。',
		gelembung: ['うーん…QRISって', '何だろう？'],
		tabsAria: 'QRISに関する5W1Hの質問',
		pertanyaan: '質問 {n} / {total}',
		tahu: 'ご存じですか？',
		ulangi: '最初からやり直す',
		lanjut: '次へ：{k}',
		prev: '前の質問',
		cta: 'EraPayでビジネスにQRISを導入する',
		items: [
			{
				k: 'なに',
				tanya: 'QRISとは？',
				isi: 'QRIS（Quick Response Code Indonesian Standard）は、インドネシア銀行とインドネシア決済システム協会（ASPI）が定めた国家統一のQRコード規格です。ひとつのQRISコードで、どのモバイルバンキングアプリやデジタルウォレットからも支払えます。',
				fakta: [['施行', '2020年1月1日'], ['規制当局', 'インドネシア銀行'], ['準拠規格', '国際規格EMVCo']],
				kartu: [],
				langkah: [],
				tahu: '以前はアプリごとに専用のQRがあり、レジに何枚ものQRステッカーを貼る必要がありました。QRISがそれをひとつにまとめました。'
			},
			{
				k: 'だれ',
				tanya: '誰が使うの？',
				isi: 'みんなです！買い手はQRIS対応の決済アプリがあればOK。売り手もどんな人でも使えます。',
				fakta: [],
				kartu: [['買い手', 'モバイルバンキングやデジタルウォレットを持つ人なら誰でも。'], ['商店・中小事業者', '市場の露店、食堂、カフェから最新の店舗まで。'], ['オンライン事業', 'ネットショップ、チケット、デジタル請求書。'], ['社会団体', '礼拝所や財団での寄付。']],
				langkah: [],
				tahu: '加盟店はEDC端末が不要。QRISステッカーと、通知を確認するスマホがあれば十分です。'
			},
			{
				k: 'いつ',
				tanya: 'いつ使える？',
				isi: 'いつでも、1日24時間・週7日、祝日も含めて利用できます。支払いは即時に処理され、買い手も売り手もすぐに成功を確認できます。',
				fakta: [['サービス時間', '24時間365日'], ['確認', 'リアルタイム']],
				kartu: [['朝', '市場で野菜を買う。'], ['昼', '食堂でランチ。'], ['夜', 'カフェでくつろぐ。']],
				langkah: [],
				tahu: 'QRIS決済はすぐに「成功」になりますが、加盟店口座への入金は事業者の精算スケジュールに従います。'
			},
			{
				k: 'どこ',
				tanya: 'どこで使える？',
				isi: 'サバンからメラウケまで、インドネシア全土のQRISマークを掲示している加盟店で利用できます。実店舗でもオンラインのチェックアウトでも使えます。',
				fakta: [],
				kartu: [['実店舗', 'QRISステッカーやレジ画面のQRをスキャン。'], ['オンライン', '支払いページにQRISが表示されます。'], ['海外', 'タイ、マレーシア、シンガポールなどの提携国で。']],
				langkah: [],
				tahu: '国際QRISなら、現金を両替せずに、インドネシアのアプリで提携国でも支払えます。'
			},
			{
				k: 'なぜ',
				tanya: 'なぜQRISを使うの？',
				isi: '買い手には手軽に、売り手にはすっきりと。',
				fakta: [],
				kartu: [['買い手', '現金を持ち歩かず、どのアプリひとつでOK。取引の証明も残ります。'], ['売り手', 'お釣りの用意も偽札の心配も不要で、取引は自動で記録されます。']],
				langkah: [],
				tahu: 'QRISの取引記録は事業の履歴となり、中小事業者が融資を申し込む際に役立ちます。'
			},
			{
				k: 'どうやって',
				tanya: 'どうやって支払うの？',
				isi: 'たった4ステップ、1分もかかりません：',
				fakta: [],
				kartu: [],
				langkah: ['モバイルバンキングまたはデジタルウォレットのアプリを開く', 'レジのQRISコードをスキャンする', '加盟店名と金額を確認する', 'PINを入力して、支払い完了！'],
				tahu: 'PINを入力する前に必ず加盟店名を確認し、PINは絶対に他人に教えないでください。'
			}
		]
	},
	tentang: {
		tag: 'EraPayについて',
		judul: ['金融システムのように設計された', '決済インフラ', '。'],
		sub: '単なるQRではありません。すべての1ルピアはサーバー検証を通り、複式簿記で記録され、取引から精算レポートまで追跡できます。',
		ledger: {
			tag: '元帳',
			judul: '複式簿記・追記専用',
			isi: 'お金は浮動小数点ではなく整数で保存。残高は変更できない仕訳から算出し、訂正は削除ではなく取消仕訳で行います。',
			jurnal: 'journal · payment.succeeded · gross',
			ilustrasi: '（イメージ）',
			akun: '勘定',
			debit: '借方',
			kredit: '貸方',
			total: '合計',
			akunJurnal: ['ゲートウェイ清算', '加盟店への債務', 'サービス手数料収益'],
			mdr: '清算前にゲートウェイが控除 · 最終料率は契約に準拠'
		},
		aman: {
			tag: 'Security by design',
			judul: '後付けではなく、多層防御',
			daftar: [['AES-256-GCM', '認証情報と書類は保存時に暗号化'], ['HMAC-SHA256', '署名付きWebhook + リプレイ対策'], ['テナント分離', 'merchant_idは入力ではなくID情報から取得'], ['MFA + メーカー・チェッカー', '出金には別々の2人が必要'], ['RBAC', '役割ごとの最小権限'], ['監査ログ', '重要な操作はすべて記録']]
		},
		idem: { tag: 'Idempotency', judul: 'リクエストが重複しても、取引はひとつ。', isi: 'キャッシュのロックだけでなく、PostgreSQLのユニーク制約で保証します。', retry: 'リトライ', sama: '（同一）' },
		state: {
			tag: 'ステートマシン',
			judul: '最終ステータスは公式ソースからのみ',
			isi1: ['成功は、検証済みのゲートウェイWebhookまたは公式な照会からのみ確定します。タイムアウトは ', ' になり、失敗と推測せず、確認してから確定します。'],
			isi2: '',
			ket: '決済・精算・返金は、それぞれ独立したステートマシンを持ちます。'
		},
		adapter: {
			tag: 'ゲートウェイ非依存',
			judul: 'APIはひとつ、背後にアダプタ',
			aria: 'POSからEraPay API、アダプタ、ゲートウェイへ',
			lapis: ['POS／お客様のアプリ', 'EraPay公開API', 'ゲートウェイアダプタ', '決済事業者'],
			isi: 'ゲートウェイ固有の用語や形式はアダプタで止まります。POSのコードを変更する必要はありません。'
		},
		stack: { label: '使用技術', daftar: ['Go モジュラーモノリス', 'PostgreSQL · ACID', 'Redis', 'Outbox ワーカー', 'REST + JSON', 'TLS everywhere'] }
	},
	cara: {
		tag: '仕組み',
		judul: 'QRISのスキャンから口座への入金まで、すべての工程を監査可能',
		sub: 'EraPayのサーバーがすべての支払いで実行する5つの段階です。段階を選ぶと、その裏で何が起きているかを確認できます。',
		daftar: '加盟店登録',
		nodeAria: '関与するコンポーネント',
		tabAria: '決済の段階',
		node: ['お客様のPOS', 'EraPay API', 'QRISゲートウェイ', '元帳', '加盟店口座'],
		contoh: 'イメージ例 · 値とIDは伏せています',
		langkah: [
			{
				t: '決済を作成',
				d: 'POSは単一のエンドポイントを呼び出します。手数料はサーバーが計算し、金額に合わせた動的QRを生成します。',
				tag: ['REST', 'Idempotency-Key', 'Bearer API key'],
				kode: [['→ POST /v1/payments', 'k'], ['  Authorization: Bearer sk_••••••••', 'm'], ['  Idempotency-Key: 7c1f…e92a', 'm'], ['  { "merchant_reference": "INV-1042", "amount": 25000,'], ['    "currency": "IDR", "payment_method": "QRIS" }'], [''], ['← 201 Created', 'ok'], ['  { "id": "pay_8Hq2…", "status": "pending",'], ['    "fee": { "mdr_fee": 175, "platform_fee": 75,', 'e'], ['             "merchant_net": 24750 },', 'e'], ['    "qr_code": { "type": "provider_defined", … } }']]
			},
			{
				t: 'お客様がスキャンして支払い',
				d: 'お客様は好きなアプリでQRISをスキャンし、取引金額ちょうどを支払います。',
				tag: ['動的QRIS', '金額固定'],
				kode: [['10:21:04  レジ画面にQRを表示', 'm'], ['10:21:04  status = pending   amount = 25000 IDR'], ['10:21:19  お客様がアプリでQRをスキャン', 'm'], ['10:21:23  QRIS事業者が支払いを処理', 'm'], ['10:21:23  ゲートウェイからの公式通知を待機中…', 'w'], [''], ['# お客様のスマホの「成功」画面はまだ確定ではない', 'c'], ['# 最終ステータスはサーバーからのみ。次のステップへ', 'c']]
			},
			{
				t: '最終ステータスの検証',
				d: 'ゲートウェイの通知は、署名・金額・重複を検証してからステータスを更新します。',
				tag: ['署名検証', '重複排除', '照会フォールバック'],
				kode: [['→ POST /webhooks/{gateway}', 'k'], ['  ✓ 署名が有効', 'ok'], ['  ✓ amount 25000 = 25000', 'ok'], ['  ✓ 未処理のイベント（重複排除）', 'ok'], ['  pay_8Hq2…  pending → succeeded', 'e'], [''], ['# タイムアウト／あいまいな応答？', 'c'], ['  status → unknown → ゲートウェイへ公式照会', 'w'], ['  # 「失敗」と推測することはない', 'c']]
			},
			{
				t: '元帳と通知',
				d: 'ステータス、複式簿記の仕訳、Webhookイベントを、1つのデータベーストランザクション（outbox）で書き込みます。',
				tag: ['ACID', 'Outbox', 'HMAC-SHA256', 'リトライ ×8'],
				kode: [['BEGIN;', 'k'], ['  journal  Dr ゲートウェイ清算      24.825', 'e'], ['           Cr 加盟店への債務        24.750', 'e'], ['           Cr サービス手数料収益        75', 'e'], ['  event    payment.succeeded  (outbox)'], ['COMMIT;   Σ 借方 = Σ 貸方 ✓', 'ok'], [''], ['→ POST https://your-pos.example/webhook', 'k'], ['  X-EraPay-Signature: t=1760600483,v1=5f2a…', 'm'], ['← 200 OK   attempt 1/8', 'ok']]
			},
			{
				t: '照合と出金',
				d: '精算レポートを元帳と照合し、その後、出金を作成して別々の2人が承認します。',
				tag: ['照合', 'メーカー・チェッカー', 'MFA'],
				kode: [['reconcile settlement-report.csv', 'k'], ['  pay_8Hq2…  gross 25.000  ✓ matched', 'ok'], ['  加盟店残高: pending → available', 'e'], [''], ['payout po_31xK…  amount = 利用可能残高', 'k'], ['  作成     : Finance   (maker)', 'm'], ['  承認     : Owner     (checker, MFA ✓)', 'ok'], ['  口座     : ••••4821  (allowlist ✓)', 'm'], ['  状態     : 加盟店口座へ送金済み', 'e']]
			}
		]
	},
	fitur: {
		tag: '主な機能のご紹介',
		judul: '現代の決済のための完全なソリューション',
		sub: '決済のことは任せて、お客様への対応に集中できます。QRISの受付から口座への入金まで、必要なものがひとつのプラットフォームに。',
		ringkas: [['6', '統合モジュール'], ['1', 'POS向けAPI'], ['3', '独立したステータス']],
		daftar: [
			{ t: 'QRIS決済', d: '動的QRを発行し、ステータスをリアルタイムで確認、通知をお客様のシステムに受け取れます。', poin: ['有効期限付きで金額に合わせたQR', '署名付きWebhook + 自動リトライ'] },
			{ t: '取引ダッシュボード', d: 'すべての取引を検索・絞り込み。決済・精算・返金のステータスは別々に表示されます。', poin: ['ステータス・日付・店舗で絞り込み', '決済ごとの完全なタイムライン'] },
			{ t: '精算と照合', d: 'ゲートウェイのレポートと元帳を突き合わせ、例外にフラグを付け、差異の原因まで追跡します。', poin: ['精算レポートの取り込み', '差異は記録され追跡可能'] },
			{ t: '出金と加盟店残高', d: '残高は元帳から算出し、出金は冪等で、すべての払い出しに承認が必要です。', poin: ['保留・利用可能・留保の残高', '承認時にメーカー・チェッカー + MFA'] },
			{ t: '返金・キャンセル・紛争', d: '専用の権限と多層の承認を備えた返金フローです。', poin: ['権限のある役割のみ', '元帳と監査ログに記録'] },
			{ t: '加盟店・店舗・役割', d: 'KYCオンボーディング、複数店舗、加盟店ごとのAPIキーとWebhook。', poin: ['オーナー・レジ担当・経理・運用', '役割に応じたアクセス権'] }
		]
	},
	cta: { judul: 'コーヒーは5秒で買えるのに、QRISを待つのは5分…お客様もイライラ！', isi: '今日から取引をスムーズに！登録してデータを入力し、システムをテストして、ドラマなしで本番開始。', daftar: '加盟店登録', masuk: 'ダッシュボードへ' }
};

export const lp: Record<Lang, Lp> = { id, en, ja };
