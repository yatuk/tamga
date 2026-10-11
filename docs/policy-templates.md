# Policy templates

> These files are starting points, not legal advice. Loading one does not
> make a deployment compliant with a law or a regulation. Each supports
> holding back the identifiers it lists, for traffic that goes through the
> proxy; what else in a text is personal data, a customer secret or a trade
> secret, and whether a request may be sent to a provider at all, is for the
> organisation and a qualified person to decide.

Four policies an operator can copy and change, in
[`proxy/policies/templates/`](../proxy/policies/templates/). Each begins
with a comment that says which sources it follows; the sources, with what
was and was not verified, are in
[research/turkish-dlp-landscape.md](research/turkish-dlp-landscape.md).

| File | For | Identifiers of a person | Contact and business identifiers | Other |
|---|---|---|---|---|
| `kvkk-baseline.yaml` | An organisation that holds personal data under Law 6698 | `BLOCK`: TC kimlik (and a foreigner's 99… number), card, Turkish passport | `REDACT`: e-mail, phone, IBAN, VKN, MERSİS, IMEI, vehicle plate, date of birth, public IP | Answers are read |
| `bddk-bank.yaml` | A bank, or a supplier handling a bank's data | `BLOCK`: TC kimlik, card, IBAN, VKN, MERSİS, Turkish passport, and two example patterns for a customer number and an account number | `REDACT`: e-mail, phone, IMEI, vehicle plate, date of birth, IP addresses | Answers are read, streamed ones too; `data.hash_findings` is on |
| `public-sector.yaml` | A public body, a municipality, a university | `BLOCK`: TC kimlik, card, Turkish passport; a text that carries "ÇOK GİZLİ" or "HİZMETE ÖZEL" in capitals | `REDACT`: e-mail, phone, IBAN, VKN, MERSİS, IMEI, vehicle plate, date of birth, IP addresses | Answers are read |
| `software-team.yaml` | Developers and coding agents | `REDACT`, not block: TC kimlik, card, IBAN, Turkish passport | `REDACT`: e-mail, phone, VKN, MERSİS, IMEI. IP addresses and plates are left alone | Every credential is `BLOCK`; an injection in a tool result is `STRIP`, elsewhere `BLOCK`; higher rate limits |

All four block secrets (`secret_detection`, sensitivity low) and injections.

## Using one

Copy the file to where `TAMGA_POLICY_PATH` points, or paste it into the
policy editor of the console, then change it:

```bash
cp proxy/policies/templates/kvkk-baseline.yaml /etc/tamga/tamga-policy.yaml
```

Things to change before relying on one:

- **Your own identifiers.** A customer number, an account number, a file
  number and a staff number have a format only you know. The patterns under
  `custom_entities` in the bank template match the words "müşteri no" and
  "hesap no" followed by digits; they are examples.
  ([operations: custom entities](operations.md#custom-entities))
- **The hash key** for `bddk-bank.yaml`. With `data.hash_findings: true` and
  neither `TAMGA_FINDING_HASH_KEY` nor `TAMGA_VAULT_KEY` set, nothing is
  stored for a finding's match and a person's rows cannot be found by a
  national id. ([operations: what a stored request holds](operations.md#what-a-stored-request-holds))
- **Notifications.** The templates have no `notify` entries. Add them to
  the rules you want to be told about, as the shipped policy does.
- **Providers.** `providers.allowed` is commented out in the bank template.
- **Limits.** `rate_limit` is the chat default except in the software
  template; `body_limits` are set only there.

## What the templates do not do

- They do not recognise a person's name, a postal address, or the special
  categories of Law 6698 Md. 6 (health, religion, union membership,
  criminal record and the rest) written as prose. There is no detector for
  them; the research page says why no keyword signal was added.
- They do not know what "müşteri sırrı" is. A customer's account history in
  a sentence passes the bank template.
- `REDACT` masks the value a pattern found and forwards the rest of the
  text. That is not anonymisation: what is left can still identify someone.
- The four Turkish identifiers added with these templates are found only
  where the text names them: `mersis_no` next to "MERSİS", `imei` next to
  "IMEI", `passport_tr` after "pasaport" or "passport" on the same line,
  `license_plate_tr` next to "plaka" or "plate". A bare number of the same
  shape is not reported. The passport pattern has no check digit to hold it
  to, and its format was not confirmed from an official source.
- The software template blocks a request that says `curl https://` or
  `wget http`, like the shipped policy does: those phrases are `tool_fetch`
  injection findings today. Watch the `BLOCK` events if your developers'
  prompts hold shell commands.
- A value found only inside an encoded block (base64, hex) has no place to
  mask, and a `REDACT` for it blocks the request
  ([operations: what REDACT and STRIP can act on](operations.md#what-redact-and-strip-can-act-on)).

## How they are tested

`proxy/internal/policy/templates_test.go` loads every file in the directory,
holds it to the validator without a single issue, checks that every category
a rule names is one a scanner reports, and runs each template against texts
with test identifiers through the real scanners. A template that stops
loading, or that names a category no scanner emits, fails
`go test ./internal/policy`. A file added to the directory has to be added
to the list in that test.
