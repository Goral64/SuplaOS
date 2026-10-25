# SuplaOS

[Po polsku](docs/pl/README.md)

An operating system for Raspberry Pi 4/5, x86-64 computers and virtual
machines ([Proxmox, VirtualBox, VMware, Hyper-V](docs/virtual-machines.md)), built around
[supla-docker](https://github.com/SUPLA/supla-docker), i.e. your own Supla
server at home. Write the image to an SD card or a disk, power the device on
and do the rest from a web browser. Besides Supla you can add extras: an MQTT
broker, Zigbee2MQTT, Zigbee2Supla, SUPLA Scripts.

The project is at an early stage. The Raspberry Pi image runs on a Pi 4
with an SSD and on a Pi 3B+ (Supla is fine there, but with 1 GB of RAM I
wouldn't add much more), it still needs testing on a Pi 5. The x86-64 image
runs on an HP T630 thin client. If you can help, see [testing](docs/testing.md).
Images are in [Releases](https://github.com/Goral64/SuplaOS/releases).

## First start

Connect the device to your router with a cable, it gets its address by
DHCP. Open https://suplaos.local:8443 in a browser (http://suplaos.local
takes you there, the IP address is also shown on a screen connected to the
device) and log in with the password `supla`. The panel asks you to change it first, the new
one also works for SSH (`ssh supla@suplaos.local`, which asks for a new
password too if you log in there first). Then you pick the language, the
time zone and the name of the device (and Wi-Fi, if the device has it), and
you can install Supla right away. Wi-Fi can also be set up later in
System, Network, after that the cable isn't needed.

Without a cable it works too, if the device has Wi-Fi: when it finds no
network for 2 minutes, it makes its own, `SuplaOS-XXXX` with the password
`suplaos-setup`. Connect a phone to it, the panel opens on its own (or open
https://192.168.4.1:8443), and pick your Wi-Fi. The device then joins it
and the phone goes back to your network. The same happens later when the
device loses its network, e.g. after the Wi-Fi password is changed in the
router.

The browser warns about the certificate at first, because the device makes
its own. Accept it, or better install the CA certificate of the device once
(System, Access in the panel, [how](docs/certificate.md)). Plain HTTP isn't
used, also not in the local network.

With your own domain (or a free one from DuckDNS or deSEC) the device can
get a Let's Encrypt certificate instead, trusted without installing
anything, see [own domain](docs/domain.md).

## Add-ons

Add-ons are installed, configured and updated in the panel, under Add-ons.
After installing Supla, Supla Cloud is at https://<device address>. The
first account can be given already when installing Supla (also in the first
run wizard), or created later on the Supla add-on page. The Zigbee2MQTT
interface is at https://<device address>:8080, served by the panel after
logging in to it.

An account on the public Supla Cloud can be moved here with its devices,
scenes and measurement history, the devices and channels keep their IDs:
see [moving from Supla Cloud](docs/migration.md).

[SUPLA Scripts](https://github.com/fracz/supla-scripts) is at
https://<device address>:8444. You log in there with a personal access
token from Supla Cloud (Account, Security, Personal access tokens) with
access to the account, devices and channels, and then make a login and a
password for SUPLA Scripts itself. It works with the Cloud on this device and
with cloud.supla.org. The image is built by SuplaOS from the releases of
supla-scripts.

With Supla on the same device there is also Connect with Supla Cloud on the
SUPLA Scripts add-on page: the panel makes an OAuth app in the Cloud (with
the account you give, the password isn't saved) and from then on SUPLA
Scripts offers logging in with Supla Cloud and gets changes of the channels
right away. Without Supla on the device the settings of the add-on take an
OAuth app made by hand (Integrations, My OAuth apps, redirect address
https://<device address>:8444/authorize).

The same can be done from SSH with `suplaos`:

    suplaos list
    suplaos info supla
    suplaos install supla
    suplaos run supla console supla:user:create

For ZigBee with a USB coordinator:

    suplaos install mosquitto
    suplaos install zigbee2mqtt ZIGBEE_ADAPTER=/dev/serial/by-id/usb-... ZIGBEE2MQTT_USB_ADAPTER=zstack
    suplaos install zigbee2supla Z2S_SUPLA_EMAIL=you@example.com

Settings can be changed later with `suplaos config <add-on> KEY=VALUE`.
Everything lives in `/srv/suplaos`: the generated compose project in
`stack/`, database dumps in `backups/`.

Supla Cloud instructions often use commands like
`docker compose exec -u www-data supla-cloud php bin/console ...`, for
example to change account limits. In SuplaOS the same goes through
`suplaos run supla console`:

    suplaos run supla console supla:user:change-limits you@example.com

`suplaos run supla console list` shows all console commands. SQL goes to
the database container with `suplaos run supla sql`, as a script or a
single query, and without arguments it opens the MariaDB client:

    suplaos run supla sql < script.sql
    suplaos run supla sql -e "select email from supla_user"

Other `docker compose` commands work as `suplaos compose ...` (e.g.
`suplaos compose ps`), in the SuplaOS compose project.

## Backups

A full backup on the Backups page holds all add-ons with their data and the
CA of the device, encrypted with a password. It can be made automatically
every day or week, also to a network share (SMB or NFS). To move to another
device or a fresh SD card, restore it in the setup wizard there. See
[full backup](docs/backup.md).

Backups of the Supla database alone are on the Supla add-on page, and the
panel makes one before every update of Supla. Restore loads one back: the
current data is backed up first, and if loading fails, it goes back in.

Over SSH:

    suplaos backup supla
    suplaos restore supla supla-20261024141039.sql.gz

## Updates

The panel shows available updates under Updates and installs them with one
button: Debian packages, the Raspberry Pi kernel and SuplaOS itself, which
comes from its own apt repository. Over SSH it's `sudo suplaos upgrade`.
Debian security and stable updates are also installed automatically every
day (it can be turned off on the same page); the panel reminds you to
restart the device when a new kernel is waiting.

When an update brings new versions of add-on files (e.g. a newer
supla-docker), the panel applies them on its own after the update, with
a backup first, and their containers are recreated. Until version 1.0 this
happens without asking. Containers of add-ons get newer images with Update
on the add-on page. Every few hours the panel checks the registries for them
and shows the versions, the new ones and a link to their changes.

What changed in each version is in [CHANGELOG.md](CHANGELOG.md).

The `suplaos` package from that repository is a part of the images. It
isn't meant for installing on an existing Debian system: the panel expects
the `supla` account and ports 80 and 8443, and the package brings Docker
from Debian.

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
`SUPLAOS_HYPERV_DIR`), creates the `SuplaOS-dev` virtual machine (another
name: `SUPLAOS_HYPERV_VM`) on the first external switch and starts it.
Windows asks for admin rights. The VM gets its address from the router, so
`https://suplaos.local:8443` works like with a real device. Every run replaces the VM disk with a fresh image.

## License

GPL-3.0, see [LICENSE](LICENSE).
