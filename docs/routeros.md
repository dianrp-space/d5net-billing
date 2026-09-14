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

## Isolir (IP → Web Proxy)

### URL page isolir

Set **Portal base URL** di admin → Settings → **Template Isolir**. Web Proxy mengarahkan ke
halaman yang menampilkan **template isolir** yang diedit di menu itu:

```
{portal_base_url}/api/public/isolir
```

Contoh: `https://billing.example.com/api/public/isolir`

Template berisi tombol login (`{{login_url}}`) yang mengarah ke halaman login/bayar isolir:

```
{portal_base_url}/isolir
```

### Alur

1. Tagihan unpaid lewat `due_date + isolir_grace_days` (Pengaturan → Umum / Cronjob; default 0 = pada jatuh tempo) → subscription `suspended`.
2. PPP/hotspot secret dipindah ke **profil isolir** (enabled), comment diawali `ISOLIR `.
3. Session di-disconnect → user reconnect mendapat IP dari **pool isolir**.
4. HTTP dari pool isolir masuk **Web Proxy** → redirect ke halaman template isolir
   (RouterOS 7: `action=redirect` + `action-data`; v6: `deny` + `redirect-to`).
5. User lihat template → klik login → masuk ke portal isolir → lihat tagihan → bayar.
6. Setelah tidak ada tunggakan past due → resume profil normal, prefix `ISOLIR ` dihapus.

### Sync otomatis (Web Proxy)

Pilih **router** + **IP pool** (dari IPAM, sudah terikat router) di Settings → Template Isolir, lalu **Sync ke router terpilih**.

Sync membuat di router itu saja:

1. `/ip/pool` dari network IPAM  
2. `/ppp/profile` (dan hotspot profile) isolir  
3. **`/ip/proxy`** + access allow, lalu redirect URL isolir (ROS7 `action=redirect` / ROS6 `redirect-to`)  
4. NAT transparent tcp/80 → proxy 8080 + filter DNS/portal, lalu drop semua trafik isolir lain (`d5n-isolir:block`)  

Comment rule: `d5n-isolir:*` (aturan lama `drp-isolir:*` otomatis di-rename saat Sync)

### Contoh manual (Winbox: IP → Web Proxy)

```
/ip pool add name=isolir ranges=10.250.0.2-10.250.0.254
/ppp profile add name=isolir local-address=10.250.0.1 remote-address=isolir rate-limit=1M/1M

/ip proxy set enabled=yes port=8080

/ip proxy access
add src-address=10.250.0.0/24 dst-host=billing.example.com action=allow comment=d5n-isolir:proxy-allow-portal
add src-address=10.250.0.0/24 action=redirect \
  action-data="https://billing.example.com/api/public/isolir" comment=d5n-isolir:proxy-redirect
# RouterOS 6:
# add src-address=10.250.0.0/24 action=deny \
#   redirect-to="https://billing.example.com/api/public/isolir" comment=d5n-isolir:proxy-redirect

/ip firewall nat
add chain=dstnat src-address=10.250.0.0/24 protocol=tcp dst-port=80 \
  action=redirect to-ports=8080 comment=d5n-isolir:nat-to-proxy

/ip firewall address-list
add list=d5n-isolir-portal address=billing.example.com comment=d5n-isolir:portal

/ip firewall filter
add chain=forward src-address=10.250.0.0/24 protocol=udp dst-port=53 action=accept comment=d5n-isolir:dns
add chain=forward src-address=10.250.0.0/24 protocol=tcp dst-port=53 action=accept comment=d5n-isolir:dns-tcp
add chain=forward src-address=10.250.0.0/24 dst-address-list=d5n-isolir-portal protocol=tcp dst-port=80,443 \
  action=accept comment=d5n-isolir:portal
add chain=forward src-address=10.250.0.0/24 action=drop comment=d5n-isolir:block
```

**Urutan rule wajib**: `block` harus berada **setelah semua rule accept** (`dns`, `dns-tcp`, `portal`) karena firewall memakai first-match. Kalau `block` nyempil di antara rule accept, DNS bisa ter-drop dan klien tidak bisa resolve domain portal (semua tampak terblokir). Sync otomatis memindahkan rule `block` ke posisi setelah accept terakhir.

Catatan:
- Web Proxy hanya mengintercept HTTP (port 80). HTTPS ke host billing harus di-allow di filter supaya halaman isolir/login bisa load.
- Rule `block` men-drop semua trafik forward lain dari pool isolir (termasuk HTTPS/port lain), sehingga user hanya bisa DNS + halaman isolir. Tanpa rule ini trafik lain lolos (default policy `forward` = accept) dan user masih bisa internet.
- Kalau web proxy gagal di-setup (mis. paket/command tidak tersedia), sync tetap menambahkan rule `block` supaya isolasi tetap jalan; error web proxy dilaporkan terpisah.
- Jangan isi IP publik di `dst-address` — pakai FQDN di address-list (RouterOS resolve A/AAAA sendiri, aman untuk Cloudflare/CDN).
