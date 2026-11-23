# Getting started

[Po polsku](pl/start.md)

From an empty card to the first device in Supla. Each part is described in
more detail in the [README](../README.md).

## 1. Hardware

- Raspberry Pi 4 (best with an SSD on USB instead of an SD card) or
  Raspberry Pi 5. A Pi 3B+ works too, but with 1 GB of RAM it's enough for
  Supla alone.
- An x86-64 computer, e.g. a mini PC or a thin client like the HP T630.
- A virtual machine: Proxmox, VirtualBox, VMware, Hyper-V, see
  [virtual machines](virtual-machines.md).

To start, it's simplest to connect the device to the router with a cable.

## 2. Image

Download from [Releases](https://github.com/Goral64/SuplaOS/releases):

- `suplaos-<version>-rpi-arm64.img.xz` for a Raspberry Pi,
- `suplaos-<version>-amd64.img.xz` for an x86-64 computer and a virtual
  machine.

## 3. Writing it

Write the image to an SD card, a USB drive or an SSD with Raspberry Pi
Imager ("Choose OS", then "Use custom") or balenaEtcher. Both take the
`.xz` file without unpacking. In Raspberry Pi Imager skip the "OS
customisation", SuplaOS doesn't use it. Writing erases everything that was
on the drive.

## 4. First start and the panel

Put the drive in, connect the network cable, power the device on and wait
2-3 minutes: on the first start the system grows to the whole disk.

Open https://suplaos.local:8443 in a browser. Not every phone and computer
knows `.local` names. Then take the IP address from the list of devices in
the router or from a screen connected to the device, and open
`https://<IP address>:8443`.

The browser warns about the certificate, as the device makes its own.
Accept it for now; later you can install the
[CA certificate of the device](certificate.md) and the warnings go away.

Log in with the password `supla`. The panel asks for a new one at once,
which then works for SSH too (`ssh supla@suplaos.local`).

Without a cable: a device with Wi-Fi that finds no network for 2 minutes
makes its own, `SuplaOS-XXXX` with the password `suplaos-setup`. Connect a
phone to it, the panel opens by itself (or open https://192.168.4.1:8443),
and choose your Wi-Fi there.

## 5. Setup wizard

You choose the language, time zone and name of the device. The address of
the panel follows the name: with the name `home` it is
https://home.local:8443.

If you're moving from another device with SuplaOS, restore its
[full backup](backup.md) here.

## 6. Supla and the account

In the last step of the wizard install Supla (or later in Add-ons, Supla).
Downloading the images takes a few minutes, depending on the connection.
You can give the first account right away, e-mail and password. The account
doesn't need a confirmation by e-mail. It can also be made later on the
page of the Supla add-on.

Supla Cloud is then at `https://<device address>`, without a port. Log in
there with that account.

Already have an account in the public Supla Cloud? You can move it with
the devices and the history, see [move from Supla Cloud](migration.md). Do
it right away, before you connect devices here.

## 7. The app and the first device

A new account has the registration of clients (apps on phones) and
devices enabled for the first week. After that a new phone gets added to
the account only once the registration of clients is on in Supla Cloud,
on the Smartphones page with the "Client registration" button. It then
stays on for 24 hours.

In the SUPLA app on the phone give the e-mail of the account in your Supla
Cloud, and in the advanced settings of the account the server address: the
IP address of SuplaOS. The app and the devices connect to the server on
port 2016. Away from home the app works only with an
[own domain](domain.md) and port 2016 forwarded.

The simplest way to add a device is the wizard in the app ("Add device"),
once the app is connected to your server. The wizard only handles a
device that:

- is in configuration mode and then has its own Wi-Fi network with a name
  starting with `SUPLA-` (or a vendor prefix, e.g. `ZAMEL-`) and ending
  with a part of its MAC address, as GUI-Generic and other firmware on
  SuplaDevice and the modules of vendors working with Supla do,
- has the standard Supla configuration page at 192.168.4.1 (Wi-Fi, server,
  e-mail),
- connects to your Wi-Fi on a band it supports: ESP8266 and most ESP32
  only 2.4 GHz.

The registration of devices doesn't need turning on in Cloud: when it is
off, the wizard asks for the password of the account and turns it on
itself. The wizard sets Wi-Fi, the server address (the one the app is
connected to) and the e-mail in the device. It doesn't change the
certificate verification: modules that have Supla CA by default (e.g.
Zamel) then won't connect to any own server. After the wizard put such a
module into configuration mode again and set the verification as below.

Without the app the registration of devices has to be on: after the
first week of the account turn it on in Supla Cloud on the My SUPLA page
with the "I/O devices registration" button (it stays on for 24 hours).
Then in the settings of the device (the configuration page of
GUI-Generic and other firmware on SuplaDevice, usually at 192.168.4.1 in
configuration mode) set:

- server: the IP address of SuplaOS in your network,
- e-mail: the e-mail of the account in your Supla Cloud,
- certificate verification: Custom CA with the certificate from the card
  "CA certificate for devices" on the page of the Supla add-on in the
  panel, or Skip certificate verification if the firmware has no Custom CA.

After saving, the device should show up in Supla Cloud within a few
seconds.

## 8. Backups

On the Backups page turn on the automatic full backup, best to a network
share, or download it to a computer now and then. A backup on the same card
won't help when the card dies. See [full backup](backup.md).

## When something doesn't work

- The panel doesn't open: check the IP address in the router and remember
  `https://` and the port `:8443`. Give the device a few minutes after the
  first start.
- A job in the panel failed: its window shows the log, and the add-on page
  has "Show logs" of its containers. The end of the log usually says what
  went wrong.
- A device doesn't connect to Supla: check the e-mail, the registration of
  devices in Cloud and the certificate verification in the device.
- The app doesn't connect: check the server address, the e-mail and the
  registration of clients in Cloud.

When asking on the forum or in
[Issues](https://github.com/Goral64/SuplaOS/issues), say what you install
on, which version of SuplaOS (Updates page), at which step you got stuck
and what exactly you see, best with the end of the log.
