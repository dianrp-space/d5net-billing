# RouterOS setup

Enable API on the router:

```
/ip service set api disabled=no port=8728
/ip service set api-ssl disabled=no port=8729
```

Create a limited API user (write needed for provisioning):

```
/user group add name=billing policy=read,write,api,!local
/user add name=drp group=billing password=CHANGE_ME
```

In drp-billing, add router with host or DDNS name and port 8728.

## Isolir (IP → Web Proxy)

### URL page isolir

Set **Portal base URL** di admin → Settings → **Template Isolir**, lalu gunakan:

```
{portal_base_url}/{tenantSlug}/client
```

Contoh: `https://billing.example.com/acme/client`

HTML fallback: `GET /api/public/tenants/{slug}/isolir`

### Alur

1. Tagihan unpaid lewat `due_date + grace_days` → subscription `suspended`.
2. PPP/hotspot secret dipindah ke **profil isolir** (enabled), comment diawali `ISOLIR `.
3. Session di-disconnect → user reconnect mendapat IP dari **pool isolir**.
4. HTTP dari pool isolir masuk **Web Proxy** → redirect ke URL isolir di atas
   (RouterOS 7: `action=redirect` + `action-data`; v6: `deny` + `redirect-to`).
5. User login singkat → lihat tagihan → bayar di portal.
6. Setelah tidak ada tunggakan past due → resume profil normal, prefix `ISOLIR ` dihapus.

### Sync otomatis (Web Proxy)

Pilih **router** + **IP pool** (dari IPAM, sudah terikat router) di Settings → Template Isolir, lalu **Sync ke router terpilih**.

Sync membuat di router itu saja:

1. `/ip/pool` dari network IPAM  
2. `/ppp/profile` (dan hotspot profile) isolir  
3. **`/ip/proxy`** + access allow, lalu redirect URL isolir (ROS7 `action=redirect` / ROS6 `redirect-to`)  
4. NAT transparent tcp/80 → proxy 8080 + filter DNS/portal  

Comment rule: `drp-isolir:*`

### Contoh manual (Winbox: IP → Web Proxy)

```
/ip pool add name=isolir ranges=10.250.0.2-10.250.0.254
/ppp profile add name=isolir local-address=10.250.0.1 remote-address=isolir rate-limit=1M/1M

/ip proxy set enabled=yes port=8080

/ip proxy access
add src-address=10.250.0.0/24 dst-host=billing.example.com action=allow comment=drp-isolir:proxy-allow-portal
add src-address=10.250.0.0/24 action=redirect \
  action-data="https://billing.example.com/acme/client" comment=drp-isolir:proxy-redirect
# RouterOS 6:
# add src-address=10.250.0.0/24 action=deny \
#   redirect-to="https://billing.example.com/acme/client" comment=drp-isolir:proxy-redirect

/ip firewall nat
add chain=dstnat src-address=10.250.0.0/24 protocol=tcp dst-port=80 \
  action=redirect to-ports=8080 comment=drp-isolir:nat-to-proxy

/ip firewall address-list
add list=drp-isolir-portal address=billing.example.com comment=drp-isolir:portal

/ip firewall filter
add chain=forward src-address=10.250.0.0/24 protocol=udp dst-port=53 action=accept comment=drp-isolir:dns
add chain=forward src-address=10.250.0.0/24 dst-address-list=drp-isolir-portal protocol=tcp dst-port=80,443 \
  action=accept comment=drp-isolir:portal
```

Catatan: Web Proxy hanya mengintercept HTTP (port 80). HTTPS ke host billing harus di-allow di filter supaya halaman isolir/login bisa load. Jangan isi IP publik di `dst-address` — pakai FQDN di address-list (RouterOS resolve A/AAAA sendiri, aman untuk Cloudflare/CDN).
