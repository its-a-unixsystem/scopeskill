[🇩🇪 Deutsche Version](README.de.md) | [🇬🇧 English Version](README.md)

# Scopeskill

Ein Claude/Codex KI-Skill (inklusive Helper-Client), um das Buchhaltungssystem [Scopevisio](https://www.scopevisio.com/) anzubinden und zu automatisieren.

## Quickstart

### Technischen Benutzer in Scopevisio anlegen

Weisen Sie dem Benutzer die erforderlichen Lizenzen und Berechtigungen zu.

### Binary herunterladen

Die kompilierte Datei finden Sie unter [Latest Release](https://github.com/its-a-unixsystem/scopeskill/releases/tag/v0.9).

> [!NOTE]
> Das Binary lässt sich auch lokal bauen:
>
> ```bash
> go build -o ./bin/sv-cli ./cmd/sv-cli
> ```

### Authentifizierung

Wählen Sie eine der folgenden Authentifizierungsmethoden.

Der interaktive Login ist zwar bequemer, erfordert aber die Eingabe Ihres Passworts. Wenn Sie das Token lieber selbst generieren möchten, können Sie das direkt über die Scopevisio-Webseite erledigen.

### Token über Scopevisio generieren
Folgen Sie der [Scopevisio REST-API-Anleitung](https://help.scopevisio.com/de/articles/467358-rest-api-erste-schritte) und generieren Sie die Tokens über die Swagger-UI oder via `curl`. Die Antwort enthält ein kurzlebiges `access_token` sowie ein langlebiges `refresh_token`.

Anschließend schreiben Sie die Konfiguration:

```bash
sv-cli auth import
```

Das Tool fragt nach Ihrer Kundennummer und dem Refresh-Token und speichert die Konfiguration.

### Interaktiver Login mit Zugangsdaten

Führen Sie den einmaligen Login aus:

```bash
sv-cli auth login
```

Der Befehl fragt folgende Daten ab:

1. Kundennummer — erforderlich
2. Benutzername — erforderlich
3. Passwort — erforderlich (wird bei der Eingabe ausgeblendet)
4. Organisations-ID — optional

Das Tool speichert `CUSTOMER` und `REST_REFRESH_TOKEN` in der aktiven Scopeskill-Konfiguration und ermittelt den verwendeten Kontenrahmen (`SKR`) automatisch.

> [!IMPORTANT]
> Benutzername, Passwort oder Organisations-ID werden niemals lokal gespeichert.

### Authentifizierung prüfen

Überprüfen Sie den Status:

```bash
sv-cli auth show
```

Suchen Sie nach Kontakten mithilfe des `sv-cli`-Helpers:

```bash
sv-cli kontakt search --email="@example.com"
```

Eine vollständige Liste aller verfügbaren Buchhaltungs-, Teamwork- und REST-API-Befehle von `sv-cli` finden Sie in der **[CLI-Referenz](docs/cli-reference.md)**.

> [!NOTE]
> `sv-cli` ist speziell für agentenbasierte Buchhaltung konzipiert, weshalb die Ausgabe standardmäßig im JSON-Format erfolgt:
>
> ```bash
> $ bin/sv-cli sachkonto search --number-prefix=1800
> [
>   {
>     "accountTypeName": "Aktiv/Passiv",
>     "active": true,
>     "name": "Bank",
>     "number": "1800"
>   }
> ]
> ```

## SKILL

Legen Sie die Dateien des Repositories im `skills`-Verzeichnis Ihres lokalen Projekts ab (z. B. `.agents/skills` oder `.claude/skills`).

Am einfachsten geht das mit dem Vercel Skills-Tool:

```bash
npx skills add https://github.com/its-a-unixsystem/scopeskill --skill scopeskill
```

## Erste Schritte

Stellen Sie sicher, dass Ihr Agent den Skill korrekt geladen hat. Danach können Sie einfach Fragen stellen:

```bash
Was sind die letzten 10 Buchungen auf dem Konto 1800?
```

```bash
Bitte liste alle gebuchten Rechnungen von Google auf und prüfe, ob sie korrekt sind.
```

## Konfigurationsdetails

Die Konfiguration ist denkbar übersichtlich:

```ini
# scopeskill config — managed by 'sv-cli auth login'
CUSTOMER=12345678
REST_REFRESH_TOKEN=aaaaaaaa-bbbbbbb-cccc-dddd-eeee-ffffffffffff
SKR=skr04
```

Standardmäßig verwendet `sv-cli` das Konfigurationsverzeichnis des jeweiligen Benutzers. Übergeben Sie `--config <path>` oder setzen Sie die Umgebungsvariable `SCOPESKILL_CONFIG`, um eine andere Datei zu verwenden.

Dauerhafte Konfigurationsschlüssel:

- `CUSTOMER`: Die Scopevisio-Kundennummer, passend zum Refresh-Token.
- `REST_REFRESH_TOKEN`: Das dauerhafte Token, um neue REST-Access-Tokens zu beziehen.
- `SKR`: Der verwendete Standardkontenrahmen (z. B. `skr03` oder `skr04`).
- `BASE_URL`: Optionaler Override der Scopevisio REST Base-URL.

Unterstützte Umgebungsvariablen (überschreiben die Config für den aktuellen Prozess):

- `SCOPESKILL_CONFIG`
- `SCOPESKILL_REST_REFRESH_TOKEN`
- `SCOPESKILL_BASE_URL`
- `SCOPESKILL_ACCESS_TOKEN_CACHE`

`SCOPESKILL_CUSTOMER` wird ganz bewusst nicht unterstützt. Wenn Sie die Identität wechseln möchten, nutzen Sie `--config` oder `SCOPESKILL_CONFIG`. Nur so ist sichergestellt, dass `CUSTOMER` und `REST_REFRESH_TOKEN` immer als Paar zusammenbleiben. Als Bearer-Header kommt stets `Authorization` zum Einsatz; es gibt weder einen `AUTH_HEADER`-Konfigurationsschlüssel noch eine entsprechende Umgebungsvariable.

REST-Access-Tokens sind nur kurz gültig. `sv-cli` speichert sie in einem separaten, flüchtigen Cache, indiziert durch den Fingerabdruck des Refresh-Tokens. Die REST-Refresh-Tokens hingegen sind dauerhafte Anmeldeinformationen in der Konfiguration. Ein Löschen des Access-Token-Caches setzt die Einrichtung nicht zurück; erst das Entfernen von `REST_REFRESH_TOKEN` aus der Config löscht das Setup.

### Speicherort der Konfiguration

| Betriebssystem | Standardpfad                                                                             |
|----------------|------------------------------------------------------------------------------------------|
| Linux          | `$XDG_CONFIG_HOME/scopeskill/config`, Fallback auf `~/.config/scopeskill/config`          |
| macOS          | `~/Library/Application Support/scopeskill/config`                                          |
| Windows        | `%AppData%\scopeskill\config` (typischerweise `C:\Users\<benutzer>\AppData\Roaming\scopeskill\config`) |

## Referenz

Die Scopevisio REST-API ist hier dokumentiert:

- https://help.scopevisio.com/de/articles/467358-rest-api-erste-schritte
- https://appload.scopevisio.com/static/swagger/index.html#/

Die Swagger-UI bezieht ihre Daten von:

- https://appload.scopevisio.com/rest/swagger.json

## Skill-Aufbau

- `SKILL.md`: Der Trigger und Betriebsleitfaden (SOP) für Agenten.
- `docs/cli-reference.md`: Referenz und Anwendungsbeispiele für `sv-cli`.
- `references/auth.md`: Dokumentation des Token- und Login-Workflows.
- `references/bookkeeping.md`: Mapping der Scopevisio-Buchhaltungsobjekte sowie API-Guardrails.
- `references/teamworkbridge.md`: Workflow für Zugriff, Upload und Download via Teamwork/CenterDevice.
- `references/workflows/`: Schritt-für-Schritt SOPs (z. B. Eingangsrechnungen, Kontobewegungen, Abstimmungen).
- `internal/scopeskill/`: Helper-Client und Konfigurations-Package für `sv-cli`.


## Lizenz

[AGPL-3.0](LICENSE)
