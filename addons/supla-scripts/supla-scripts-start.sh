#!/bin/sh
# Start of supla-scripts in SuplaOS: config.json from the add-on settings,
# then the entrypoint of the image (migrations, Apache and cron).
set -e

var=/var/www/var
mkdir -p "$var/backups" "$var/cache" "$var/config" "$var/logs" "$var/ssl" "$var/system"
# supla-scripts dumps its database on every start and keeps them all; the
# backups of the add-on are made by SuplaOS, the five newest are enough.
ls -t "$var"/backups/*.sql.gz 2>/dev/null | tail -n +6 | xargs -r rm -f

cat > "$var/config/config.json" <<EOF
{
  "displayErrorDetails": false,
  "readOnly": false,
  "apiCache": true,
  "clearLogsOlderThan": "1week",
  "phpPath": "/usr/local/bin/php",
  "intervals": {"thermostat": 5, "clearLogs": 60},
  "requestQuota": {"perIpPerMinute": 60},
  "failuresToDisable": {"thermostat": 60},
  "db": {
    "driver": "mysql",
    "database": "suplascripts",
    "host": "supla-scripts-db",
    "username": "suplascripts",
    "password": "$SUPLA_SCRIPTS_DB_PASSWORD",
    "charset": "utf8",
    "prefix": ""
  },
  "jwt": {
    "key": "$SUPLA_SCRIPTS_JWT_KEY",
    "expirationTime": 3600,
    "expirationTimeRememberMe": 604800,
    "expirationTimeClient": 157680000,
    "iss": "https://scripts.supla.io"
  },
  "metrics": {"enabled": false, "instanceName": "root", "host": "metrics", "statsd_port": 8125}
}
EOF
chmod 640 "$var/config/config.json"

# supla-scripts and its libraries are older than PHP 8.2: without this the
# log gets deprecation notices from every cron run.
echo 'error_reporting = E_ALL & ~E_DEPRECATED & ~E_USER_DEPRECATED' > /usr/local/etc/php/conf.d/zz-suplaos.ini

exec /usr/local/bin/docker-php-entrypoint "$@"
