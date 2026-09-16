import { useEffect, useState } from "react";
import { ArrowLeft, Mail, MapPin, Phone } from "lucide-react";
import { api } from "./api";
import { applyBrandingMeta, DEFAULT_BRAND_LOGO } from "./branding";

type PublicSite = {
  name?: string;
  app_name?: string;
  logo_url?: string | null;
  favicon_url?: string | null;
  support_email?: string;
  support_phone?: string;
  support_address?: string;
};

type Block = { h: string; p?: string[]; ul?: string[] };

const UPDATED = "16 September 2026";

function termsBlocks(name: string): Block[] {
  return [
    {
      h: "1. Penerimaan Ketentuan",
      p: [
        `Syarat & Ketentuan ini ("Ketentuan") mengatur penggunaan situs, portal pelanggan, dan layanan internet yang disediakan oleh ${name} ("Kami").`,
        "Dengan mengakses situs atau menggunakan layanan Kami, Anda menyatakan telah membaca, memahami, dan menyetujui seluruh Ketentuan ini. Apabila Anda tidak menyetujui, mohon untuk tidak menggunakan layanan.",
      ],
    },
    {
      h: "2. Definisi",
      ul: [
        '"Layanan" berarti jasa akses internet dan/atau portal pelanggan untuk melihat tagihan dan melakukan pembayaran.',
        `"Penyedia" berarti ${name}.`,
        '"Pelanggan" atau "Anda" berarti pengguna layanan yang terdaftar.',
        '"Akun" berarti kredensial akses portal yang diberikan kepada Pelanggan.',
      ],
    },
    {
      h: "3. Layanan",
      p: [
        "Kami menyediakan jasa layanan internet (ISP) beserta portal pelanggan untuk memeriksa paket, tagihan, riwayat pembayaran, dan pengajuan keluhan.",
        "Nama paket, kecepatan, dan harga dalam Rupiah (IDR) ditampilkan pada halaman produk di situs Kami dan dapat berubah sewaktu-waktu. Harga yang berlaku bagi Anda adalah harga yang tercantum pada tagihan yang diterbitkan.",
      ],
    },
    {
      h: "4. Akun dan Keamanan",
      p: [
        "Anda bertanggung jawab menjaga kerahasiaan data akun (nomor telepon/email dan password) dan seluruh aktivitas yang terjadi melalui Akun Anda.",
        "Segera hubungi Kami apabila mengetahui adanya penggunaan Akun tanpa izin atau dugaan pelanggaran keamanan.",
      ],
    },
    {
      h: "5. Pembayaran dan Tagihan",
      p: [
        "Tagihan diterbitkan secara berkala sesuai siklus berlangganan dan/atau atas transaksi tertentu. Jatuh tempo tercantum pada masing-masing tagihan.",
        "Pembayaran dapat dilakukan melalui metode yang tersedia, termasuk namun tidak terbatas pada transfer/VA bank, e-wallet, gerai retail, dan QRIS melalui penyedia payment gateway.",
        "Pembayaran dianggap sah setelah dikonfirmasi oleh sistem Kami. Bukti pembayaran dari pihak ketiga yang belum terkonfirmasi sistem bukan merupakan bukti pelunasan.",
      ],
    },
    {
      h: "6. Keterlambatan dan Isolir",
      p: [
        "Tagihan yang melewati jatuh tempo dapat dikenai denda keterlambatan sesuai ketentuan yang berlaku.",
        "Kami berhak melakukan pembatasan atau isolir (penangguhan) sementara atas layanan apabila terdapat tunggakan, sampai dengan pembayaran dilunasi.",
        "Layanan akan dipulihkan setelah pembayaran dikonfirmasi, dalam waktu yang wajar.",
      ],
    },
    {
      h: "7. Penggunaan yang Dilarang",
      p: ["Anda dilarang menggunakan Layanan untuk:"],
      ul: [
        "tindakan yang melanggar hukum, termasuk penyebaran konten ilegal;",
        "mengakses, mengubah, atau mengganggu sistem, jaringan, atau perangkat milik Kami maupun pihak lain tanpa izin;",
        "mengirim spam, malware, atau melakukan serangan yang mengganggu layanan;",
        "menjual kembali atau mengalihkan Layanan tanpa persetujuan tertulis dari Kami.",
      ],
    },
    {
      h: "8. Hak Kekayaan Intelektual",
      p: [
        `Seluruh merek, logo, nama, konten, dan perangkat lunak yang terkait dengan Layanan merupakan milik ${name} atau pemberi lisensinya, dan dilindungi oleh hukum yang berlaku.`,
      ],
    },
    {
      h: "9. Penangguhan dan Penghentian",
      p: [
        "Kami dapat menangguhkan atau menghentikan Layanan apabila Anda melanggar Ketentuan ini, memiliki tunggakan, atau berdasarkan permintaan otoritas yang berwenang.",
        "Anda dapat mengajukan penghentian berlangganan sesuai prosedur yang berlaku, dengan menyelesaikan seluruh kewajiban yang masih tertunggak.",
      ],
    },
    {
      h: "10. Batasan Tanggung Jawab",
      p: [
        'Layanan disediakan "sebagaimana adanya". Kami berupaya menjaga ketersediaan dan kualitas layanan, namun tidak menjamin bahwa Layanan akan selalu bebas dari gangguan.',
        "Sejauh diizinkan oleh hukum, Kami tidak bertanggung jawab atas kerugian tidak langsung, insidental, atau konsekuensial yang timbul dari penggunaan atau ketidakmampuan menggunakan Layanan.",
      ],
    },
    {
      h: "11. Perubahan Ketentuan",
      p: [
        "Kami dapat memperbarui Ketentuan ini dari waktu ke waktu. Versi terbaru akan dipublikasikan pada halaman ini dan berlaku sejak tanggal pembaruan.",
        "Dengan terus menggunakan Layanan setelah perubahan, Anda dianggap menyetujui Ketentuan yang diperbarui.",
      ],
    },
    {
      h: "12. Hukum yang Berlaku",
      p: [
        "Ketentuan ini diatur dan ditafsirkan berdasarkan hukum Republik Indonesia. Sengketa yang timbul akan diselesaikan secara musyawarah terlebih dahulu, dan apabila tidak tercapai, melalui pengadilan yang berwenang.",
      ],
    },
  ];
}

function privacyBlocks(name: string): Block[] {
  return [
    {
      h: "1. Pendahuluan",
      p: [
        `Kebijakan Privasi ini menjelaskan bagaimana ${name} ("Kami") mengumpulkan, menggunakan, menyimpan, dan melindungi data pribadi Anda ketika Anda menggunakan situs, portal pelanggan, dan layanan internet Kami.`,
        `Dengan menggunakan Layanan, Anda menyetujui praktik yang dijelaskan dalam Kebijakan Privasi ini. Kebijakan ini disusun dengan memperhatikan ketentuan perlindungan data pribadi yang berlaku di Indonesia, termasuk Undang-Undang Perlindungan Data Pribadi.`,
      ],
    },
    {
      h: "2. Data yang Kami Kumpulkan",
      p: ["Kami dapat mengumpulkan data berikut:"],
      ul: [
        "Data identitas, seperti nama, nomor telepon, email, alamat, dan data identitas lain yang diperlukan untuk pendaftaran;",
        "Data langganan dan perangkat, seperti paket, alamat pemasangan, dan informasi perangkat jaringan;",
        "Data tagihan dan pembayaran, seperti riwayat tagihan, nominal, dan metode pembayaran;",
        "Data teknis, seperti alamat IP, log akses, dan informasi perangkat yang Anda gunakan untuk mengakses Layanan.",
      ],
    },
    {
      h: "3. Cara Kami Menggunakan Data",
      ul: [
        "menyediakan, mengoperasikan, dan memelihara Layanan;",
        "menerbitkan tagihan, memproses pembayaran, dan melakukan penagihan;",
        "memberikan dukungan dan menangani keluhan atau permintaan Anda;",
        "menjaga keamanan sistem serta mencegah penyalahgunaan dan penipuan;",
        "memenuhi kewajiban hukum dan ketentuan regulator yang berlaku.",
      ],
    },
    {
      h: "4. Dasar Hukum Pemrosesan",
      p: [
        "Kami memproses data pribadi berdasarkan persetujuan Anda, pelaksanaan perjanjian layanan, pemenuhan kewajiban hukum, serta kepentingan sah Kami yang tidak bertentangan dengan hak Anda.",
      ],
    },
    {
      h: "5. Berbagi Data dengan Pihak Ketiga",
      p: [
        "Kami dapat membagikan data kepada pihak ketiga yang diperlukan untuk penyediaan Layanan, antara lain:",
      ],
      ul: [
        "penyedia layanan pembayaran (payment gateway) untuk memproses transaksi pembayaran;",
        "mitra teknis dan penyedia infrastruktur yang mendukung operasional jaringan dan sistem;",
        "aparat penegak hukum atau instansi berwenang, apabila diwajibkan oleh peraturan perundang-undangan.",
        "Kami tidak menjual data pribadi Anda kepada pihak lain.",
      ],
    },
    {
      h: "6. Cookie dan Teknologi Serupa",
      p: [
        "Situs dan portal Kami dapat menggunakan cookie atau penyimpanan lokal untuk menjaga sesi login dan menyimpan preferensi Anda. Anda dapat mengatur penggunaan cookie melalui pengaturan peramban (browser), namun sebagian fitur mungkin tidak berfungsi optimal bila cookie dinonaktifkan.",
      ],
    },
    {
      h: "7. Penyimpanan dan Keamanan Data",
      p: [
        "Kami menyimpan data pribadi selama akun Anda aktif atau selama diperlukan untuk tujuan yang dijelaskan di atas dan/atau untuk memenuhi kewajiban hukum.",
        "Kami menerapkan langkah teknis dan organisasi yang wajar untuk melindungi data pribadi dari akses, penggunaan, atau pengungkapan yang tidak sah. Meskipun demikian, tidak ada metode transmisi atau penyimpanan data yang sepenuhnya bebas risiko.",
      ],
    },
    {
      h: "8. Hak Anda",
      p: ["Sesuai ketentuan yang berlaku, Anda berhak untuk:"],
      ul: [
        "meminta akses dan salinan data pribadi Anda;",
        "meminta pembaruan atau perbaikan data yang tidak akurat;",
        "meminta penghapusan data dalam keadaan tertentu;",
        "menarik persetujuan atas pemrosesan data, sepanjang tidak bertentangan dengan kewajiban hukum Kami.",
      ],
    },
    {
      h: "9. Data Anak",
      p: [
        "Layanan Kami tidak ditujukan untuk anak di bawah usia yang ditentukan oleh hukum. Kami tidak dengan sengaja mengumpulkan data pribadi anak tanpa persetujuan orang tua atau wali yang sah.",
      ],
    },
    {
      h: "10. Perubahan Kebijakan Privasi",
      p: [
        "Kami dapat memperbarui Kebijakan Privasi ini dari waktu ke waktu. Versi terbaru akan dipublikasikan pada halaman ini beserta tanggal pembaruan.",
      ],
    },
  ];
}

export function LegalPage({ doc }: { doc: "terms" | "privacy" }) {
  const [site, setSite] = useState<PublicSite | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      let loaded: PublicSite | null = null;
      try {
        loaded = await api<PublicSite>("/api/public/site");
      } catch {
        try {
          const b = await api<PublicSite>("/api/public/branding");
          loaded = { name: b.name, app_name: b.app_name, logo_url: b.logo_url, favicon_url: b.favicon_url };
        } catch {
          /* keep defaults */
        }
      }
      if (cancelled || !loaded) return;
      setSite(loaded);
      applyBrandingMeta({ appName: loaded.name || loaded.app_name, faviconUrl: loaded.favicon_url });
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const name = (site?.name || site?.app_name || "Penyedia Layanan").trim();
  const logo = site?.logo_url || DEFAULT_BRAND_LOGO;
  const email = (site?.support_email || "").trim();
  const phone = (site?.support_phone || "").trim();
  const address = (site?.support_address || "").trim();
  const hasContact = Boolean(email || phone || address);

  const isTerms = doc === "terms";
  const title = isTerms ? "Syarat & Ketentuan" : "Kebijakan Privasi";
  const blocks = isTerms ? termsBlocks(name) : privacyBlocks(name);

  return (
    <div className="min-h-screen bg-[var(--bg)] text-[var(--text)]">
      <header className="border-b border-[var(--border)] bg-[var(--panel)]/85 backdrop-blur-md">
        <div className="mx-auto flex max-w-4xl items-center justify-between gap-3 px-6 py-4">
          <a href="/" className="flex items-center gap-3 no-underline" title="Ke halaman utama" aria-label="Ke halaman utama">
            <img src={logo} alt={name} className="h-9 w-auto" />
            <span className="text-lg font-bold">{name}</span>
          </a>
          <a href="/" className="inline-flex items-center gap-1.5 text-sm font-medium text-[var(--muted)] hover:text-[var(--accent)]">
            <ArrowLeft size={15} /> Beranda
          </a>
        </div>
      </header>

      <main className="mx-auto max-w-3xl px-6 pb-20 pt-10">
        <p className="text-xs font-semibold uppercase tracking-[0.2em] text-[var(--muted)]">Dokumen legal</p>
        <h1 className="mt-2 text-3xl font-extrabold tracking-tight sm:text-4xl">{title}</h1>
        <p className="mt-2 text-sm text-[var(--muted)]">Terakhir diperbarui: {UPDATED}</p>

        <div className="mt-8 grid gap-7">
          {blocks.map((b) => (
            <section key={b.h}>
              <h2 className="text-base font-bold text-[var(--text)]">{b.h}</h2>
              {b.p?.map((para, i) => (
                <p key={i} className="mt-2 text-sm leading-7 text-[var(--muted)]">
                  {para}
                </p>
              ))}
              {b.ul ? (
                <ul className="mt-2 list-disc space-y-1.5 pl-5 text-sm leading-7 text-[var(--muted)]">
                  {b.ul.map((li, i) => (
                    <li key={i}>{li}</li>
                  ))}
                </ul>
              ) : null}
            </section>
          ))}

          <section>
            <h2 className="text-base font-bold text-[var(--text)]">{`${blocks.length + 1}. Kontak`}</h2>
            <p className="mt-2 text-sm leading-7 text-[var(--muted)]">
              Apabila Anda memiliki pertanyaan mengenai {isTerms ? "Syarat & Ketentuan" : "Kebijakan Privasi"} ini,
              silakan hubungi Kami:
            </p>
            {hasContact ? (
              <ul className="mt-3 grid gap-2 text-sm text-[var(--text)]">
                {email ? (
                  <li className="flex items-center gap-2">
                    <Mail size={15} className="text-[var(--accent)]" />
                    <a href={`mailto:${email}`} className="font-medium hover:text-[var(--accent)]">
                      {email}
                    </a>
                  </li>
                ) : null}
                {phone ? (
                  <li className="flex items-center gap-2">
                    <Phone size={15} className="text-[var(--accent)]" />
                    <a href={`tel:${phone.replace(/[^+\d]/g, "")}`} className="font-medium hover:text-[var(--accent)]">
                      {phone}
                    </a>
                  </li>
                ) : null}
                {address ? (
                  <li className="flex items-start gap-2">
                    <MapPin size={15} className="mt-0.5 text-[var(--accent)]" />
                    <span className="font-medium">{address}</span>
                  </li>
                ) : null}
              </ul>
            ) : (
              <p className="mt-2 text-sm leading-7 text-[var(--muted)]">
                Informasi kontak belum tersedia. Silakan hubungi administrator layanan.
              </p>
            )}
          </section>
        </div>
      </main>

      <footer className="border-t border-[var(--border)]">
        <div className="mx-auto flex max-w-3xl flex-wrap items-center justify-between gap-3 px-6 py-6 text-xs text-[var(--muted)]">
          <span>
            © {new Date().getFullYear()} {name}
          </span>
          <nav className="flex flex-wrap items-center gap-4">
            <a href="/terms" className="hover:text-[var(--accent)]">
              Syarat &amp; Ketentuan
            </a>
            <a href="/privacy" className="hover:text-[var(--accent)]">
              Kebijakan Privasi
            </a>
            <a href="/" className="hover:text-[var(--accent)]">
              Beranda
            </a>
          </nav>
        </div>
      </footer>
    </div>
  );
}
