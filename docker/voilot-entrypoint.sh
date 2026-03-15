#!/bin/sh
# voilot-entrypoint.sh — Generate server certificate signed by the build-time CA,
# then start nginx. Runs on every container start so the cert always matches
# the current LAN IP (passed via VOILOT_TLS_SAN env var).
#
# The CA certificate (/etc/nginx/ssl/ca.crt) is baked into the image at build
# time and never changes — install it once on your mobile device to trust all
# server certs this CA signs.

set -e

SSL_DIR=/etc/nginx/ssl

# Build the SAN list: always include localhost, add user-supplied SANs
SAN="DNS:localhost,IP:127.0.0.1"
if [ -n "$VOILOT_TLS_SAN" ]; then
	SAN="${SAN},${VOILOT_TLS_SAN}"
fi

echo "[voilot] Generating server certificate with SANs: ${SAN}"

# Create a temporary OpenSSL config for the server cert
cat >/tmp/server-ext.cnf <<EOF
[req]
distinguished_name = req_dn
req_extensions = v3_req
prompt = no

[req_dn]
CN = voilot
O = voilot-dev

[v3_req]
subjectAltName = ${SAN}

[v3_ext]
subjectAltName = ${SAN}
EOF

# Generate server key + CSR
openssl genrsa -out "${SSL_DIR}/voilot.key" 2048 2>/dev/null

openssl req -new \
	-key "${SSL_DIR}/voilot.key" \
	-out /tmp/voilot.csr \
	-config /tmp/server-ext.cnf

# Sign with the build-time CA
openssl x509 -req \
	-in /tmp/voilot.csr \
	-CA "${SSL_DIR}/ca.crt" \
	-CAkey "${SSL_DIR}/ca.key" \
	-CAcreateserial \
	-out "${SSL_DIR}/voilot.crt" \
	-days 825 \
	-extfile /tmp/server-ext.cnf \
	-extensions v3_ext \
	2>/dev/null

# Clean up temp files
rm -f /tmp/voilot.csr /tmp/server-ext.cnf

echo "[voilot] Server certificate ready. Starting nginx..."

exec nginx -g "daemon off;"
