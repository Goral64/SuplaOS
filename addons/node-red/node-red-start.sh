#!/bin/sh
# Runs as root in the container: hands /data to the node-red user (the
# directory is made by SuplaOS, files may come back from a backup), then
# starts Node-RED as that user.
set -eu

# First start with the Supla nodes in the image: a flow with the Supla
# connection that takes its settings from the add-on (SUPLA_TOKEN and
# SUPLA_MQTT_* in the environment).
if [ ! -e /data/flows.json ] && [ -d /usr/src/node-red/node_modules/node-red-contrib-supla ]; then
  cat > /data/flows.json <<'FLOWS'
[
  {"id": "5e1a0000c0f10001", "type": "tab", "label": "Supla", "disabled": false, "info": ""},
  {"id": "5e1a0000c0f10002", "type": "supla-config", "name": "Supla", "fromEnv": true, "pollInterval": 10}
]
FLOWS
fi

find /data ! -user node-red -exec chown node-red:node-red {} +
export HOME=/usr/src/node-red
exec su -p -s /bin/sh node-red -c 'cd /usr/src/node-red && exec ./entrypoint.sh'
