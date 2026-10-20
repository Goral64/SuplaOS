#!/bin/sh
# Writes the Mosquitto configuration and password file from the add-on
# settings, then starts the broker. Runs as root in the container, the
# broker drops to the mosquitto user.
set -eu

conf=/mosquitto/config
mkdir -p "$conf"

: > "$conf/passwd"
mosquitto_passwd -b "$conf/passwd" "$MOSQUITTO_USERNAME" "$MOSQUITTO_PASSWORD"

{
  echo "per_listener_settings false"
  echo "allow_anonymous false"
  echo "password_file $conf/passwd"
  echo "persistence true"
  echo "persistence_location /mosquitto/data/"
  echo "log_dest stdout"
  echo "listener 1883"
  case "$MOSQUITTO_LAN" in
    tls|both)
      # The key is readable only by root on the host: copy it for the broker.
      cp /mosquitto/certs/server.crt /mosquitto/certs/server.key "$conf/"
      echo "listener 8883"
      echo "certfile $conf/server.crt"
      echo "keyfile $conf/server.key"
      ;;
  esac
} > "$conf/mosquitto.conf"

chown -R mosquitto:mosquitto "$conf" /mosquitto/data
chmod 600 "$conf/passwd"
exec mosquitto -c "$conf/mosquitto.conf"
