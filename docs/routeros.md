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

- Set port ini di Settings → **Template Isolir** → *Port captive isolir*. Nilai harus sama
  dengan port pada `ISOLIR_HTTP_ADDR`.
- **Port 8090 harus dibuka di firewall server** (`ufw allow 8090/tcp`, security group, dll).
  Ini **bukan** path nginx/HTTPS — akses uji: `http://IP_PUBLIK:8090/` (harus tampil halaman isolir).
  Kalau `https://domain:8090` atau lewat reverse proxy HTTPS, biasanya gagal (listener plain HTTP).
- **IP tujuan DST-NAT** diambil dari **address-list RouterOS** (FQDN `portal_base_url`),
  prefer IPv4 publik — sama seperti yang terlihat di Winbox. Ini menghindari DNS di server
  billing yang sering mengembalikan IP LAN (split-horizon). Opsional: isi *IP host isolir*
  di Settings untuk override manual.
- Cloudflare **DNS only** (abu-abu) OK; kalau proxied (oranye) address-list dapat IP CF, bukan server.

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
- DST-NAT hanya membelokkan HTTP (port 80). HTTPS tidak bisa di-intercept (sertifikat), tapi rule `block` men-drop-nya sehingga user tetap terisolir; saat user membuka situs HTTP apapun ia diarahkan ke halaman isolir.
- Rule `portal` juga meng-allow port captive (mis. `8090`) supaya trafik hasil dst-nat (menuju IP portal:port captive) tidak ikut ter-drop.
- Rule `block` men-drop semua trafik forward lain dari pool isolir, sehingga user hanya bisa DNS + halaman isolir. Tanpa rule ini trafik lain lolos (default policy `forward` = accept) dan user masih bisa internet.
- `to-addresses` dst-nat diisi dari IPv4 hasil resolve **address-list RouterOS** (bukan
  `LookupIP` di server billing). Override manual lewat Settings → IP host isolir.
- Untuk filter tetap pakai FQDN di address-list (RouterOS refresh A/AAAA sendiri).
