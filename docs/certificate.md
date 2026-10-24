# Certificate of the device

[Po polsku](pl/certificate.md)

The panel, Supla Cloud, Zigbee2MQTT and the encrypted MQTT port only work
over HTTPS. Every SuplaOS device makes its own certificate authority (CA) on
the first start and signs certificates for itself with it. Browsers don't
know that CA, so until you install it they warn about the connection. You can
accept the warning, but it's nicer to install the CA once on every computer
and phone you use. After that the panel, Supla Cloud and Zigbee2MQTT open
without warnings, also when the device gets another IP address.

The CA can only sign certificates for `.local` names and addresses in local
networks, so installing it doesn't let anyone pretend to be other websites.

## Download

In the panel go to System, Certificate and click Download CA certificate.
The file is called `suplaos-<name>-ca.crt`. The same card shows its SHA-256
fingerprint, compare it with the one your system shows when installing.

The certificates are valid for `https://<name>.local` and the IP address of
the device, not for the bare name without `.local`.

## Windows

1. Double-click the file, then Install Certificate.
2. Current User, Next.
3. Place all certificates in the following store, Browse, Trusted Root
   Certification Authorities.
4. Next, Finish and confirm the security warning.

Chrome and Edge use it right away. Firefox normally takes it from Windows
too; if it still warns, import the file in Firefox (see below).

## macOS

1. Double-click the file, Keychain Access adds it to the login keychain.
2. Open the certificate there (SuplaOS ...), expand Trust and set When using
   this certificate to Always Trust.
3. Close the window and confirm with your password.

## Linux

For the system and command line tools (curl, mosquitto clients) on Debian
and Ubuntu:

    sudo cp suplaos-<name>-ca.crt /usr/local/share/ca-certificates/
    sudo update-ca-certificates

Browsers on Linux keep their own list, so import the file there as well.

## Firefox

Settings, Privacy & Security, Certificates, View Certificates, Authorities,
Import. Pick the file and tick Trust this CA to identify websites.

## Android

Settings, Security (sometimes under More security settings), Encryption &
credentials, Install a certificate, CA certificate, then pick the
downloaded file. The menu names differ between phone makers, searching the
settings for "CA certificate" helps. Browsers use it, most apps don't.

## iPhone and iPad

1. Open the panel in Safari and download the certificate, iOS offers to
   download a profile.
2. Settings, Profile Downloaded, Install.
3. Settings, General, About, Certificate Trust Settings: turn on full trust
   for the SuplaOS certificate.

## MQTT

Clients connecting to the encrypted port 8883 check the broker with the same
CA, e.g. `mosquitto_sub --cafile suplaos-<name>-ca.crt ...`. The Mosquitto
add-on page also offers the file.

## New device, new CA

The CA stays the same after changing the name or the address of the device.
A freshly written image makes a new one: install it again and remove the old
one. Each CA has its own name, e.g. `SuplaOS suplaos 3fa2c1d0`, so CAs of
several devices can be installed side by side. CAs made by SuplaOS 0.4.0 and
0.4.1 were all called `SuplaOS suplaos` and browsers mixed them up (Firefox
refused to open the page); from 0.4.2 they are replaced on their own, so
install the CA once more and remove the old one.
