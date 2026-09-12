#!/bin/sh
set -eu
umask 077
mkdir -p /config
if [ ! -s /config/roomdeck.env ]; then
  key="rd$(od -An -N12 -tx1 /dev/urandom | tr -d ' \n')"
  secret="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
  printf 'LIVEKIT_API_KEY=%s\nLIVEKIT_API_SECRET=%s\n' "$key" "$secret" > /config/roomdeck.env.tmp
  mv /config/roomdeck.env.tmp /config/roomdeck.env
fi
. /config/roomdeck.env
case "${ROOMDECK_MEDIA_IP:-}" in *[!0-9a-fA-F.:]*) echo 'ROOMDECK_MEDIA_IP must be an IP address' >&2; exit 1;; esac
case "${ROOMDECK_TURN_DOMAIN:-}" in *[!a-zA-Z0-9.-]*) echo 'Invalid TURN domain' >&2; exit 1;; esac
cat > /config/livekit.yaml.tmp <<EOF
port: 7880
bind_addresses: [0.0.0.0]
rtc:
  tcp_port: 7881
  udp_port: 7882
EOF
if [ -n "${ROOMDECK_MEDIA_IP:-}" ]; then
 printf '  node_ip: "%s"\n  use_external_ip: false\n' "$ROOMDECK_MEDIA_IP" >> /config/livekit.yaml.tmp
else
 printf '  use_external_ip: true\n' >> /config/livekit.yaml.tmp
fi
cat >> /config/livekit.yaml.tmp <<EOF
room:
  auto_create: false
  empty_timeout: 60
  departure_timeout: 20
keys:
  $LIVEKIT_API_KEY: $LIVEKIT_API_SECRET
turn:
  enabled: true
  udp_port: 3478
EOF
if [ -n "${ROOMDECK_TURN_DOMAIN:-}" ]; then
 test -s /certs/fullchain.pem && test -s /certs/privkey.pem || { echo 'TURN certificates are missing' >&2; exit 1; }
 cat >> /config/livekit.yaml.tmp <<EOF
  tls_port: 443
  domain: "$ROOMDECK_TURN_DOMAIN"
  cert_file: /certs/fullchain.pem
  key_file: /certs/privkey.pem
EOF
else
 printf '  tls_port: 0\n' >> /config/livekit.yaml.tmp
fi
if [ -n "${ROOMDECK_MEDIA_IP:-}" ]; then
 case "$ROOMDECK_MEDIA_IP" in *:*) cidr="$ROOMDECK_MEDIA_IP/128";; *) cidr="$ROOMDECK_MEDIA_IP/32";; esac
 printf '  allow_restricted_peer_cidrs:\n    - "%s"\n' "$cidr" >> /config/livekit.yaml.tmp
fi
mv /config/livekit.yaml.tmp /config/livekit.yaml
chown 10001:10001 /config /config/roomdeck.env /config/livekit.yaml
chmod 750 /config
chmod 600 /config/roomdeck.env /config/livekit.yaml
echo 'Media configuration ready; keys preserved.'
