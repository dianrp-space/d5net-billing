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

Isolir / walled garden: create PPP profile `isolir` that redirects HTTP to the customer portal.
