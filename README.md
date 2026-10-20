# SuplaOS

An operating system for Raspberry Pi 4/5 and x86-64 computers, built around
[supla-docker](https://github.com/SUPLA/supla-docker), i.e. your own Supla
server at home. Write the image to an SD card or a disk, power the device on
and do the rest from a web browser. Besides Supla you can add extras: an MQTT
broker, Zigbee2MQTT, zigbee2supla.

The project is at an early stage, for now only the x86-64 image is being built.

## First start

The device gets its address from your router (DHCP). The panel will be at
http://suplaos.local (for now there is only a placeholder page), and the IP
address is also shown on a screen connected to the device.

SSH login: `ssh supla@suplaos.local`, password `supla`.
Change it with `passwd` after logging in.

## Add-ons

Until the panel is ready, add-ons are managed from SSH with `suplaos`:

    suplaos list
    suplaos info supla
    suplaos install supla
    suplaos run supla console supla:user:create

Supla Cloud is then at https://<device address> (port 80 is kept for the
panel). For ZigBee with a USB coordinator:

    suplaos install mosquitto
    suplaos install zigbee2mqtt ZIGBEE_ADAPTER=/dev/serial/by-id/usb-...
    suplaos install zigbee2supla Z2S_SUPLA_EMAIL=you@example.com

Settings can be changed later with `suplaos config <add-on> KEY=VALUE`.
Everything lives in `/srv/suplaos`: the generated compose project in
`stack/`, database dumps from `suplaos backup` in `backups/`.

## Building the image

You need Docker.

    tools/build.sh amd64

The image lands in `out/`. For a quick test in QEMU:

    tools/run-qemu.sh

SSH is then available on `localhost:2222`.

On Windows with WSL you can run the image in Hyper-V instead:

    tools/hyperv.sh

It converts the image to VHDX in `F:\Hyper-V\SuplaOS` (change it with
`SUPLAOS_HYPERV_DIR`), creates the `SuplaOS-dev` virtual machine on the first
external switch and starts it. Windows asks for admin rights. The VM gets its
address from the router, so `http://suplaos.local` works like with
a real device. Every run replaces the VM disk with a fresh image.
