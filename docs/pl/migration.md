# Przeniesienie z Supla Cloud

[English](../migration.md)

SuplaOS może przejąć Twoje konto z publicznego Supla Cloud (albo z innego
własnego serwera Supli): urządzenia, kanały z ustawieniami, lokalizacje,
identyfikatory dostępu, grupy kanałów, sceny, harmonogramy, reakcje, linki
bezpośrednie, Twoje ikony i historię pomiarów.

Urządzenia i kanały zachowują swoje ID. Aplikacje, tematy MQTT, Home
Assistant, SUPLA Scripts i Twoje skrypty, które z nich korzystają, działają
dalej, gdy tylko wskażą nowy serwer.

## Zanim zaczniesz

- Zainstaluj Suplę w SuplaOS i nie dodawaj tam jeszcze urządzeń.
  Przeniesienie wstawia obiekty z ID, jakie mają w źródle, więc Supla tutaj
  musi być świeża.
- Konto tutaj dostaje e-mail konta źródłowego (urządzenia rejestrują się tym
  adresem). Jeśli go nie ma, przeniesienie je zakłada, z podanym hasłem.
- W źródłowym Supla Cloud utwórz osobisty token dostępowy: Konto,
  Bezpieczeństwo, Osobiste tokeny dostępowe, z zakresami odczytu.

## Przenoszenie

1. Strona dodatku Supla, karta Przeniesienie z innego Supla Cloud. Wklej
   token i odczytaj konto. Zobaczysz, co zostanie przeniesione.
2. Podaj hasło konta tutaj i przygotuj przeniesienie. Wszystko oprócz samych
   urządzeń przenosi się od razu, razem z historią pomiarów do tej chwili.
   Przy długiej historii trwa to kilka minut.
3. Przestawiaj urządzenia po kolei na ten serwer: w ustawieniach urządzenia
   (GUI-Generic, SuplaDevice) wpisz adres SuplaOS jako serwer, w aplikacji
   Supla Config dla urządzeń fabrycznych, w opcjach Zigbee2Supla. E-mail
   zostaje. Żeby urządzenie sprawdzało certyfikat serwera, użyj Custom CA
   z certyfikatem ze strony Supli, zobacz [certyfikat](certificate.md).
4. Gdy urządzenie się połączy, w ciągu minuty dostaje swoje ustawienia.
   Karta pokazuje, które urządzenia są już przeniesione, a które czekają.
5. Po przestawieniu urządzeń dociągnij nowszą historię: źródło zapisuje
   pomiary, dopóki urządzenie nie zostanie przeniesione. Potem zakończ
   przenoszenie.

Dopóki urządzenia nie przestawisz, działa ze źródłem jak dotąd, a jego
stary wpis tam zostaje, więc możesz wrócić, ustawiając dawny serwer.

## Co się zmienia

- Linki bezpośrednie mają tu nowe adresy, karta je wypisuje. Zmień je tam,
  gdzie ich używasz.
- Grupy, sceny i ikony dostają nowe ID, a sceny, harmonogramy i reakcje,
  które z nich korzystają, są ustawione już z nowymi.
- Integracje źródła (Google Home, Alexa, Home Assistant przez Cloud,
  aplikacje OAuth) trzeba połączyć tutaj od nowa.
- W aplikacjach dodaj konto z adresem tego serwera.
