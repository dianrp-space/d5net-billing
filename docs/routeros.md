# RouterOS setup

Enable API on the router:

```
/ip service set api disabled=no port=8728
/ip service set api-ssl disabled=no port=8729
```

Create a limited API user (write needed for provisioning):

```
/user group add name=billing policy=read,write,api,!local
/user add name=d5n group=billing password=CHANGE_ME
```

In d5net-billing, add router with host or DDNS name and port 8728.

## Isolir (IP → DST-NAT)

Isolir memakai **DST-NAT** (bukan Web Proxy). Trafik HTTP dari pool isolir dibelokkan
langsung ke **captive listener** aplikasi (port `ISOLIR_HTTP_ADDR`, default `:8090`) yang
menampilkan halaman isolir untuk host/URL apapun. Ini tidak butuh paket web-proxy dan
jalan di RouterOS 6 maupun 7.

### URL page isolir

Set **Portal base URL** di admin → Settings → **Template Isolir**. Halaman yang menampilkan
**template isolir** yang diedit di menu itu:

```
{portal_base_url}/api/public/isolir
```

Contoh: `https://billing.example.com/api/public/isolir`

Template berisi tombol login (`{{login_url}}`) yang mengarah ke halaman login portal pelanggan:

```
{portal_base_url}/login
```

### Alur

1. Tagihan unpaid lewat `due_date + isolir_grace_days` (Pengaturan → Umum / Cronjob; default 0 = pada jatuh tempo) → subscription `suspended`.
2. PPP/hotspot secret dipindah ke **profil isolir** (enabled), comment diawali `ISOLIR `.
3. Session di-disconnect → user reconnect mendapat IP dari **pool isolir**.
4. HTTP (tcp/80) dari pool isolir di-**DST-NAT** ke `IP portal : port captive` → captive
   listener aplikasi menampilkan halaman template isolir untuk situs apapun yang dibuka.
5. User lihat template → klik login → masuk portal pelanggan → lihat tagihan → bayar.
6. Setelah tidak ada tunggakan past due → resume profil normal, prefix `ISOLIR ` dihapus.

### Captive listener aplikasi

Aplikasi menjalankan listener HTTP terpisah (env `ISOLIR_HTTP_ADDR`, default `0.0.0.0:8090`)
yang **selalu** membalas halaman isolir untuk request host/URL apapun. Inilah tujuan DST-NAT.

- Settings → **Port captive isolir** = `8090` (sama dengan port di `ISOLIR_HTTP_ADDR`).
- Pastikan port itu reachable dari router/klien (firewall server, dan kalau server di balik
  CHR/WG: forward/dst-nat tcp/8090 ke host billing).
- Uji: `http://IP_PUBLIK:8090/` harus tampil halaman isolir (plain HTTP, bukan HTTPS).
- **IP tujuan DST-NAT** dari address-list RouterOS (prefer publik), atau override *IP host isolir*.
- Cloudflare **DNS only** (abu-abu) OK; proxied (oranye) = IP CF, salah.

### Sync otomatis (DST-NAT)

Pilih **router** + **IP pool** (dari IPAM, sudah terikat router) di Settings → Template Isolir, lalu **Sync ke router terpilih**.

Sync membuat di router itu saja:

1. `/ip/pool` dari network IPAM  
2. `/ppp/profile` (dan hotspot profile) isolir  
3. **NAT dst-nat**: tcp/80 dari pool → `to-addresses=<IP portal> to-ports=<port captive>`  
4. Filter allow DNS + portal (address-list FQDN, port `80,443,<port captive>`), lalu drop semua trafik isolir lain (`d5n-isolir:block`)  

Aturan web-proxy lama (`d5n-isolir:proxy-*`) dihapus dan rule NAT lama `d5n-isolir:nat-to-proxy`
dikonversi jadi dst-nat saat Sync. Comment rule: `d5n-isolir:*` (aturan lama `drp-isolir:*` otomatis dimigrasi).

### Contoh manual (Winbox: IP → Firewall)

Misal pool isolir `10.250.0.0/24`, IP server billing `203.0.113.10`, port captive `8090`:

```
/ip pool add name=isolir ranges=10.250.0.2-10.250.0.254
/ppp profile add name=isolir local-address=10.250.0.1 remote-address=isolir rate-limit=1M/1M

/ip firewall nat
add chain=dstnat src-address=10.250.0.0/24 protocol=tcp dst-port=80 \
  action=dst-nat to-addresses=203.0.113.10 to-ports=8090 comment=d5n-isolir:nat-dstnat

/ip firewall address-list
add list=d5n-isolir-portal address=billing.example.com comment=d5n-isolir:portal

/ip firewall filter
add chain=forward src-address=10.250.0.0/24 protocol=udp dst-port=53 action=accept comment=d5n-isolir:dns
add chain=forward src-address=10.250.0.0/24 protocol=tcp dst-port=53 action=accept comment=d5n-isolir:dns-tcp
add chain=forward src-address=10.250.0.0/24 dst-address-list=d5n-isolir-portal protocol=tcp dst-port=80,443,8090 \
  action=accept comment=d5n-isolir:portal
add chain=forward src-address=10.250.0.0/24 action=drop comment=d5n-isolir:block
```

**Urutan rule wajib**: `block` harus berada **setelah semua rule accept** (`dns`, `dns-tcp`, `portal`) karena firewall memakai first-match. Kalau `block` nyempil di antara rule accept, DNS bisa ter-drop dan klien tidak bisa resolve domain portal (semua tampak terblokir). Sync otomatis memindahkan rule `block` ke posisi setelah accept terakhir.

Catatan:
- DST-NAT membelokkan HTTP (port 80) klien ke IP portal:8090 (captive). HTTPS tidak
  di-intercept; rule `block` men-drop-nya.
- Rule `portal` allow 80+443+8090 ke FQDN billing supaya login/bayar + captive tetap jalan.
- `to-addresses` dari address-list RouterOS (bukan LookupIP di server billing).
- Untuk filter tetap pakai FQDN di address-list.
