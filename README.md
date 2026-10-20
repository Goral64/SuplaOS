# SuplaOS

[Po polsku](docs/pl/README.md)

An operating system for Raspberry Pi 4/5, x86-64 computers and virtual
machines ([Proxmox, VirtualBox, VMware, Hyper-V](docs/virtual-machines.md)), built around
[supla-docker](https://github.com/SUPLA/supla-docker), i.e. your own Supla
server at home. Write the image to an SD card or a disk, power the device on
and do the rest from a web browser. Besides Supla you can add extras: an MQTT
broker, Zigbee2MQTT, zigbee2supla.

The project is at an early stage. The Raspberry Pi image hasn't been tested
on real hardware yet, if you can help, see [testing](docs/testing.md).
Images are in [Releases](https://github.com/Goral64/SuplaOS/releases).

## First start

The device gets its address from your router (DHCP). Open
http://suplaos.local in a browser (the IP address is also shown on a screen
connected to the device) and log in with the password `supla`. The panel asks
you to change it first, the new one also works for SSH
(`ssh supla@suplaos.local`, which asks for a new password too if you log in
there first). Then you pick the language, the time zone and the
name of the device, and you can install Supla right away.

## Add-ons

Add-ons are installed, configured and updated in the panel, under Add-ons.
After installing Supla, Supla Cloud is at https://<device address> (port 80
belongs to the panel) and the account can be created on the Supla add-on page.

The same can be done from SSH with `suplaos`:

    suplaos list
    suplaos info supla
    suplaos install supla
    suplaos run supla console supla:user:create

For ZigBee with a USB coordinator:

    suplaos install mosquitto
    suplaos install zigbee2mqtt ZIGBEE_ADAPTER=/dev/serial/by-id/usb-...
    suplaos install zigbee2supla Z2S_SUPLA_EMAIL=you@example.com

Settings can be changed later with `suplaos config <add-on> KEY=VALUE`.
Everything lives in `/srv/suplaos`: the generated compose project in
`stack/`, database dumps in `backups/`.

## Updates

The panel shows available updates under Updates and installs them with one
button: Debian packages, the Raspberry Pi kernel and SuplaOS itself, which
comes from its own apt repository. Over SSH it's `sudo suplaos upgrade`.
Debian security and stable updates are also installed automatically every
day (it can be turned off on the same page); the panel reminds you to
restart the device when a new kernel is waiting.

When an update brings new versions of add-on files (e.g. a newer
supla-docker), the panel lists them and applies them when you ask, after
a backup, because their containers are recreated. Containers of add-ons
get newer images with Update on the add-on page.

The `suplaos` package from that repository is a part of the images. It
isn't meant for installing on an existing Debian system: the panel expects
the `supla` account and port 80, and the package brings Docker from Debian.

## Building the image

You need Docker.

    tools/build.sh amd64
    tools/build.sh rpi

The image lands in `out/`. On an x86-64 machine the Raspberry Pi image is
built with qemu emulation, which takes a while. For a quick test in QEMU:

    tools/run-qemu.sh

SSH is then available on `localhost:2222`.

On Windows with WSL you can run the image in Hyper-V instead:

    tools/hyperv.sh

It converts the image to VHDX in `F:\Hyper-V\SuplaOS` (change it with
`SUPLAOS_HYPERV_DIR`), creates the `SuplaOS-dev` virtual machine on the first
external switch and starts it. Windows asks for admin rights. The VM gets its
address from the router, so `http://suplaos.local` works like with
a real device. Every run replaces the VM disk with a fresh image.

## License

GPL-3.0, see [LICENSE](LICENSE).
