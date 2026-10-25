# Full backup

[Po polsku](pl/backup.md)

A full backup is one file with everything needed to bring the device back
or move it to another one: the add-ons with their settings, their data
(Supla Cloud and SUPLA Scripts databases, paired Zigbee devices, Mosquitto
users...), the CA of the device and the own domain with its certificate.

The settings of the device itself are not in it: name, time zone, language,
network, Wi-Fi and the password. You set them on the new device in the
setup wizard.

## Making a backup

Backups page, Full backup. Give a password and make a backup, then download
it from the list below. Without the password the backup can't be restored,
so keep it somewhere safe. A backup without a password is possible too, but
anyone who gets the file can read the passwords of the add-ons and the key
of the CA from it.

It takes a few seconds. Mosquitto, Zigbee2MQTT and Zigbee2Supla stop for a
moment meanwhile, so that their files are copied in one piece.

The file is a `.tar.gz`, encrypted with [age](https://age-encryption.org)
when it has a password (`.tar.gz.age`). On a computer it can be opened with
`age -d backup.tar.gz.age > backup.tar.gz`.

## Automatic backups

Backups page, Automatic backups: every day or every week at a given time.
If the device is off then, the backup is made when it's on again. The
newest few are kept, older automatic ones are deleted (backups made by hand
stay).

Where to:

- The disk of the device. Good against a mistake, not against a broken SD
  card or disk.
- A network share on a computer or NAS (SMB, as in Windows). Give the share
  as `//192.168.1.10/backups`, with a user and password, or without a user
  for a guest share. If the NAS is old, pick the SMB version by hand.
- An NFS share, `192.168.1.10:/volume1/backups`. The NAS has to let the
  device write there as root (no root squash), or the directory has to
  belong to the user root is mapped to.

Check the share tests it with a small file before you save. The password of
the backups is kept on the device, the backups are encrypted at night
without you.

When a backup fails (the NAS is off, a wrong password), the card shows the
error. The next try is at the next scheduled time, or press Back up now.

## Moving to another device

1. On the old device make a full backup and download it.
2. Write SuplaOS to the new one and start it. In the setup wizard choose
   the language, time zone and name, then on the last step Restore from a
   full backup instead of installing Supla. Send the file and give the
   password.
3. The new device needs the internet: the add-ons are downloaded again.
   It takes a few minutes.
4. Turn the old device off. Two devices with the same Zigbee coordinator
   or the same Supla accounts get in each other's way.

Things that stay as they were and may need a look afterwards:

- Supla devices connect to the server address saved in them. If that was
  the IP address of the old device, give the new one the same address
  (a DHCP reservation in the router) or change it in the devices. With an
  own domain, point the name to the new device (local DNS entry or the
  record at your DNS provider).
- A Zigbee coordinator on USB may have another name on the new device. The
  log of the restore says so, set it again on the Zigbee2MQTT page.
- The CA of the device moves with the backup, so a CA certificate already
  installed on phones and computers keeps working. The certificates of the
  panel and the add-ons are made anew for the new name and address.

The new device may be another kind (a Raspberry Pi after a mini PC or the
other way round), the backup doesn't depend on it. It can't be restored on
an older SuplaOS than the one that made it: update first.

## Restoring on the same device

Backups page, Full backup, Restore next to a backup on the list, or Send a
backup to upload one from your computer. Everything the add-ons have is
replaced. A full backup of the current state is made first and shows up on
the list, so a restore can be undone.

## Over SSH

    sudo SUPLAOS_BACKUP_PASSWORD='...' suplaos export
    sudo SUPLAOS_BACKUP_PASSWORD='...' suplaos import /srv/suplaos/backups/suplaos-....tar.gz.age --yes

Without `SUPLAOS_BACKUP_PASSWORD` the backup isn't encrypted. `export -o`
writes it to another file.
