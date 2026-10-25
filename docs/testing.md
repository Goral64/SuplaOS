# Testing SuplaOS

[Po polsku](pl/testing.md)

SuplaOS is at an early stage and I'm looking for people who can try it,
especially on a Raspberry Pi 4 or 5, which I can't test myself right now.
Don't use it for your main Supla setup yet: things will change. Updates
are installed from the panel (Updates), but early versions may still need
a fresh image now and then.

## Getting the image

Images are in [Releases](https://github.com/Goral64/SuplaOS/releases):

- `suplaos-<version>-rpi-arm64.img.xz` for Raspberry Pi 4 and 5,
- `suplaos-<version>-amd64.img.xz` for x86-64 computers and virtual machines
  (UEFI, see [virtual machines](virtual-machines.md)).

Write it to an SD card, a USB disk or an SSD with Raspberry Pi Imager
("Choose OS", then "Use custom") or balenaEtcher. Both take the `.xz` file
directly. In Raspberry Pi Imager skip the "OS customisation", SuplaOS
doesn't use it.

The first start works with a network cable or over Wi-Fi: without a
network the device makes its own, `SuplaOS-XXXX` with the password
`suplaos-setup`, and the panel opens on a phone connected to it. Wi-Fi is
set up in the panel: in the setup wizard or later in System, Network.

## First start

1. Power the device on and wait 2-3 minutes. On the first start the system
   grows to the whole disk and generates its keys.
2. Find its address: on a screen connected to the device, in the list of
   devices in your router, or try http://suplaos.local (it takes you to
   https://suplaos.local:8443).
3. Open the panel in a browser and log in with the password `supla`. It asks
   you to change it. The new password is also the SSH password
   (`ssh supla@suplaos.local`).
4. Go through the first start wizard: language, time zone, device name, and
   install Supla if you want.

## What to check

- Does the device start and get an address? Is the address shown on a
  screen connected to it?
- Does the panel open, does the dashboard show sensible values (memory,
  disk, temperature on a Raspberry Pi)?
- Supla: install it with the first account (or create one on the Supla
  add-on page later), log in to
  Supla Cloud at https://<device address> and connect a Supla device.
- ZigBee (if you have a coordinator): install Mosquitto, Zigbee2MQTT and
  Zigbee2Supla. Pick the USB coordinator from the list and its type
  (zstack, ember, deconz), Zigbee2MQTT then starts without its own first
  start page.
- The certificate: install the CA from System, Access
  ([how](certificate.md)), do the panel, Supla Cloud and Zigbee2MQTT open
  without warnings?
- Restart the device from the panel (System), everything should come back
  on its own.
- When a new version comes out: update from the panel (Updates) and check
  that everything still works.

## Feedback

Write what you tested and what went wrong, with:

- the device (e.g. Raspberry Pi 4 4 GB, mini PC, Proxmox VM) and the disk
  (SD card, USB SSD, NVMe),
- the image version,
- if something didn't work, the output of these commands (over SSH):

      cat /etc/suplaos-release
      systemctl --failed
      ip -br a
      suplaos status
      sudo journalctl -b -p warning --no-pager | tail -n 100

Problems can go to [Issues](https://github.com/Goral64/SuplaOS/issues).
