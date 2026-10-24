# Changes

[Po polsku](docs/pl/CHANGELOG.md)

## 0.4.1

- Backups can be restored: Restore on the Supla add-on page or
  `suplaos restore supla <file>`. The current data is backed up first.
- Upload backup puts a downloaded backup on the device, e.g. a new one.
- Backups of the Supla database now also have its procedures.

## 0.4.0

- HTTPS only. The panel moved to https://<name>.local:8443, port 80 only
  redirects there. Supla Cloud stays on 443 and no longer listens on 8081.
  The Zigbee2MQTT interface is still on port 8080, but over HTTPS and only
  after logging in to the panel.
- The device has its own certificate authority. Install its certificate
  once (System page, [how](docs/certificate.md)) and the panel, Supla Cloud,
  Zigbee2MQTT and MQTT on port 8883 work without warnings. The certificates
  are renewed on their own when the name or the address of the device
  changes. The Cloud certificate no longer expires after a year.
- MQTT clients using the old `server.crt` of Mosquitto need the CA
  certificate instead (Mosquitto add-on page or System page).
- New add-on files brought by an update are applied on its own right after
  it (with a backup first), no need to click Apply.

After updating from an older version reload the panel page, it's now at the
new address and the browser warns about the certificate until the CA is
installed.

## 0.3.1

- The system update log opens in the same window as other operations, with
  its state and a Close button; Show the log opens it again later.
- Confirmations look like the rest of the panel instead of browser pop-ups.
- Cancel on the password change page.
- A new build of the same image version shows its build time instead of
  "26.05.01 → 26.05.01".
- New supla-docker versions are checked every day and become a pull
  request here after the tests pass with them.

## 0.3.0

- The panel checks every few hours whether add-ons have newer images. The
  add-on page shows the installed version, the new one and a link to the
  changes, and Update is only active when there is something to update.
  Add-ons with updates are marked on the dashboard, the add-on list and
  the Updates page.
- Start, Stop and Restart follow the state of the add-on.
- Updating an add-on no longer recreates the containers of other add-ons
  (updating Supla restarted zigbee2supla).
- Dashboard: uptime as days, hours and minutes, CPU count on its own line.

## 0.2.5

- `suplaos run supla sql` runs SQL in the Supla database: a script
  (`< file.sql`), a query (`-e "..."`) or the MariaDB client.
- The README shows how to run the Supla Cloud console commands, e.g. to
  change account limits.

## 0.2.4

- The first boot setup (new SSH keys, growing the partition) runs only on
  SuplaOS images, not after installing the package somewhere else.

## 0.2.3

- Polish documentation in `docs/pl`.

## 0.2.2

- The restart reminder shows up after every kernel update, also the
  automatic one, and on the dashboard too.
- Automatic Debian updates can be turned off on the Updates page.

## 0.2.1

- With the Polish system language the panel didn't see any updates.
- The update waits when apt is busy instead of failing.

## 0.2.0

- Updates of Debian, the Raspberry Pi kernel and SuplaOS itself from the
  panel (Updates) or with `sudo suplaos upgrade`. SuplaOS comes from its
  own apt repository.
- New versions of add-on files are applied on request, after a backup.
- qemu-guest-agent in the x86-64 image, notes on virtual machines.

## 0.1.0

- First images for Raspberry Pi 4/5 and x86-64, with the web panel and
  the Supla, Mosquitto, Zigbee2MQTT and zigbee2supla add-ons. No updates
  yet, install 0.2.0 or newer instead.
