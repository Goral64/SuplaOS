# Changes

[Po polsku](docs/pl/CHANGELOG.md)

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
