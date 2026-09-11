#!/bin/sh
set -eu

domain="${1:-fluxeando.test}"
output="deploy/local/certs"
mkdir -p "$output"
openssl req -x509 -newkey rsa:2048 -sha256 -days 30 -nodes \
  -keyout "$output/privkey.pem" \
  -out "$output/fullchain.pem" \
  -subj "/CN=$domain" \
  -addext "subjectAltName=DNS:$domain"
chmod 600 "$output/privkey.pem"
echo "Certificado local creado para $domain en $output."
