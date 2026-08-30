#!/bin/sh
set -eu

render() {
  template="$1"
  envsubst '$DOMAIN' < "$template" > /etc/nginx/conf.d/default.conf
}

certificate="/etc/letsencrypt/live/${DOMAIN}/fullchain.pem"
render /etc/nginx/templates/http.conf.template
nginx -g 'daemon off;' &
nginx_pid=$!

trap 'nginx -s quit; wait "$nginx_pid"; exit 0' TERM INT
while [ ! -f "$certificate" ]; do sleep 5; done

render /etc/nginx/templates/https.conf.template
nginx -s reload
while kill -0 "$nginx_pid" 2>/dev/null; do
  sleep 6h & wait $!
  nginx -s reload || true
done
wait "$nginx_pid"
