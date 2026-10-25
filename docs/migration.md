# Moving from Supla Cloud

[Po polsku](pl/migration.md)

SuplaOS can take over your account from the public Supla Cloud (or from
another own Supla server): devices, channels with their settings,
locations, access IDs, channel groups, scenes, schedules, reactions, direct
links, your icons and the measurement history.

Devices and channels keep their IDs. The apps, MQTT topics, Home Assistant,
SUPLA Scripts and your own scripts that use them keep working once they
point to the new server.

## Before you start

- Install Supla in SuplaOS and don't add devices there yet. The move puts
  the objects in with the IDs they have in the source, so the Supla here has
  to be fresh.
- The account here gets the e-mail of the source account (devices register
  with it). If there is none, the move makes it, with the password you give.
- In the source Supla Cloud make a personal access token: Account, Security,
  Personal access tokens, with the read scopes. To move SUPLA Scripts too,
  add Channels: execute actions.

## Moving

1. Supla add-on page, card Move from another Supla Cloud. Paste the token
   and read the account. You see what will be moved.
2. Give the password of the account here and prepare the move. Everything
   except the devices themselves is moved now, with the measurement history
   up to this moment. This takes a few minutes with a long history.
3. Switch the devices one by one to this server: in the settings of a device
   (GUI-Generic, SuplaDevice) set the address of SuplaOS as the server, in
   the Supla Config app for factory devices, in the options of Zigbee2Supla.
   The e-mail stays. To check the server certificate, use Custom CA with the
   certificate from the Supla page, see [certificate](certificate.md).
4. When a device connects, it gets its settings within a minute. The card
   shows which devices are moved and which are waiting.
5. After switching the devices, copy the newer history: the source keeps
   logging until a device is moved. Then end the move.

Until a device is switched, it works with the source as before, and its old
entry stays there, so you can go back by setting the old server again.

## SUPLA Scripts

If you use SUPLA Scripts and have it installed here, the card can move its
scenes, scene groups, thermostats and notifications too. They refer to
channels by ID, which the move keeps, so they work here as they did.

- An account that signs in to SUPLA Scripts only through Supla Cloud needs a
  username and password there first, otherwise the token is not let in. To
  set them, sign out of SUPLA Scripts, choose Other login methods, link the
  account with a private Supla Cloud, paste a personal token and fill in the
  username and password. Signing in through Supla keeps working.
- SUPLA Scripts asks for a token with Channels: execute actions. If the
  token you moved with has only the read scopes, make another one with it
  and paste it in the card next to the address of SUPLA Scripts.
- Objects that already exist here with the same name are skipped.
- Notifications go to the phones registered here, so register the app with
  SUPLA Scripts here before moving them.

## What changes

- Direct links get new addresses here, the card lists them. Change them
  where you use them.
- Groups, scenes and icons get new IDs, the scenes, schedules and reactions
  that use them are set up with the new ones.
- Integrations of the source (Google Home, Alexa, Home Assistant through
  the Cloud, OAuth apps) have to be connected again here.
- In the apps add an account with the address of this server.
