# Dizajn sistema — Moj Drive

## 1. Pregled arhitekture i servisi
Sistem je organizovan u tri logička servisa unutar Go backend aplikacije uz MongoDB bazu podataka, dok se za spoljašnje zahteve koristi `AuthMiddleware` kao ulazna zaštita (API gateway sloj):

- **Users / Auth servis:** registracija i prijava korisnika, upravljanje korisničkim nalozima, ulogama i kvotom prostora (`users` kolekcija).
- **Files & Folders servis:** kreiranje foldera i podfoldera, pregled sadržaja foldera, otpremanje datoteka i novih verzija, preimenovanje stavki i čuvanje fajlova (`folders` i `files` kolekcije + direktorijum za otpremljene fajlove).
- **Notifications servis:** čitanje korisničkih notifikacija (`notifications` kolekcija).

## 2. Entiteti i atributi

### User (`users`)
- `id` (`ObjectID`, BSON `_id`) — jedinstveni identifikator korisnika
- `first_name` (`string`) — ime korisnika
- `last_name` (`string`) — prezime korisnika
- `email` (`string`) — email adresa korisnika
- `password` (`string`) — lozinka korisnika
- `role` (`string`) — uloga korisnika (npr. `RK`)
- `quota` (`int64`) — maksimalna dozvoljena kvota prostora u bajtovima

### Folder (`folders`)
- `id` (`string`) — jedinstveni identifikator foldera
- `name` (`string`) — naziv foldera
- `parent_id` (`*string`, opciono) — ID roditeljskog foldera (`nil` ako je u root-u)
- `owner_id` (`string`) — ID vlasnika foldera

### File (`files`)
- `id` (`string`) — jedinstveni identifikator datoteke
- `name` (`string`) — naziv datoteke
- `folder_id` (`*string`, opciono) — ID nadređenog foldera (`nil` ako je u root-u)
- `owner_id` (`string`) — ID vlasnika datoteke
- `size` (`int64`) — veličina datoteke u bajtovima
- `updated_at` (`time.Time`) — vreme poslednje izmene / nove verzije

### Notification (`notifications`)
- `id` (`string`) — jedinstveni identifikator notifikacije
- `user_id` (`string`) — ID korisnika kome notifikacija pripada
- `message` (`string`) — tekst notifikacije
- `read` (`bool`) — status pročitanosti
- `created_at` (`time.Time`) — vreme kreiranja notifikacije

## 3. Stilovi komunikacije između servisa

- **Klijent ↔ Backend (HTTP/REST):** Klijentska aplikacija šalje HTTP zahteve (JSON i `multipart/form-data`). Zaštićene rute prolaze kroz `AuthMiddleware`, koji proverava `Authorization: Bearer <token>` zaglavlje i prosleđuje `X-User-ID`.
- **Sinhrona komunikacija između servisa (HTTP/REST):** Servisi međusobno komuniciraju sinhrono preko internih HTTP endpointa koristeći `UsersClient` i `FilesClient` (konfigurisane preko `USERS_SERVICE_URL` i `FILES_SERVICE_URL`):
  - `GET /api/internal/users/{id}/exists` — sinhrona provera da li korisnik postoji pre kreiranja foldera.
  - `GET /api/internal/users/{id}/quota` — sinhrona provera korisničke kvote pre otpremanja fajla, otpremanja nove verzije i pri pregledu kvote.
  - `GET /api/internal/files/used?user_id=...` — sinhrono dobavljanje trenutnog zauzeća prostora korisnika.
- **Asinhrona komunikacija:** Nije korišćen poseban message broker; notifikacije se čitaju direktno iz kolekcije `notifications` putem `GET /api/notifications`.