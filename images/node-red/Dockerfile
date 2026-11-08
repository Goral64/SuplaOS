# Node-RED with the Supla nodes (Goral64/SuplaNodeRED), for the node-red
# add-on of SuplaOS. Built by .github/workflows/node-red.yml.
ARG NODE_RED_VERSION=5.0.9
FROM nodered/node-red:${NODE_RED_VERSION}

ARG SUPLA_NODES_VERSION=0.1.1
# Next to Node-RED itself, not in /data: the nodes come with the image and
# are updated with it.
RUN npm install --no-audit --no-fund --omit=dev "github:Goral64/SuplaNodeRED#v${SUPLA_NODES_VERSION}" \
    && npm cache clean --force
