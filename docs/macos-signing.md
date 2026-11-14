# Signing and notarizing the macOS downloads

Signed and notarized, **Briss.app** and the **briss** command line open on a Mac like
any other download, without the "could not verify" warning and the trip to
Privacy & Security. The workflows already know how to do it
(`scripts/macos/`); they only need five repository secrets from the project's
Apple Developer account. Until those exist, the app gets an ad-hoc signature and
the command line is left as the Go linker made it, as in 2.0 beta.

You need a membership of the Apple Developer Program, the **Account Holder** role
(only the Account Holder can create Developer ID certificates and the first API
key), and a Mac. It takes about half an hour.

| Secret | What goes in it |
|---|---|
| `MACOS_SIGN_P12` | The Developer ID Application certificate and its private key, exported as `.p12`, base64-encoded |
| `MACOS_SIGN_PASSWORD` | The password you gave the `.p12` when exporting it |
| `MACOS_NOTARY_KEY` | The App Store Connect API key file (`AuthKey_XXXXXXXXXX.p8`), base64-encoded |
| `MACOS_NOTARY_KEY_ID` | That key's ID (10 characters) |
| `MACOS_NOTARY_ISSUER_ID` | The Issuer ID shown above the list of keys (a UUID) |

These are secrets: put them only into GitHub's secret settings, never into a file in
the repository, an issue, a commit message or a chat. `.gitignore` refuses `*.p12`,
`*.p8`, `*.cer` and `*.certSigningRequest` files to make accidents harder.

## 1. Find or create the Developer ID Application certificate

Two kinds of certificate look alike in Keychain Access:

- **Apple Development: *your name* (*ID*)**: Xcode makes this one by itself. It signs
  builds for your own devices; macOS rejects it on anyone else's Mac, and Apple's notary
  service refuses it.
- **Developer ID Application: *your name* (*TEAM ID*)**: the one for apps given to
  others outside the Mac App Store. Only the Account Holder can create it. This is the
  one the builds need.

### Check whether you already have it

If you made a Developer ID certificate before, it is probably still there. In Terminal:

```bash
security find-identity -v -p codesigning
```

Every line is a certificate with its private key on this Mac. A line with
**Developer ID Application** means you have it: skip to step 2 and export that one (not
the "Apple Development" one). Lines that start with **Apple Development** are not it,
whatever name or team ID they show in brackets: a Mac often has several of them, one per
team or Apple ID that Xcode has signed in with. The team ID in brackets only says which
team a certificate belongs to; the words before the colon say what kind it is.

If [developer.apple.com](https://developer.apple.com/account) lists a Developer ID
Application certificate under **Certificates** but this Mac does not show it, its private
key is on the Mac that made the request. Apple never receives the key, so downloading
the certificate again does not bring it back. Export it on that Mac, or create a new one
as below; you do not need to revoke the old one.

### Create a new one with a certificate signing request (CSR)

1. Open **Keychain Access** (in Applications → Utilities, or search for it with
   Spotlight).
2. From the menu bar choose **Keychain Access → Certificate Assistant → Request a
   Certificate From a Certificate Authority…**
3. Fill in the form:
   - **User Email Address**: the email of your Apple developer account;
   - **Common Name**: any name you will recognise, for example `Briss Developer ID`
     (it names the private key in the keychain);
   - **CA Email Address**: leave it empty;
   - **Request is**: choose **Saved to disk**.

   Click **Continue** and save `CertificateSigningRequest.certSigningRequest`, for example
   on the Desktop. At this moment Keychain Access makes the private key and keeps it in
   your login keychain; it never leaves this Mac.
4. In a browser, go to [developer.apple.com/account](https://developer.apple.com/account),
   **Certificates, IDs & Profiles → Certificates**, and click the blue **+**.
5. Under **Software**, choose **Developer ID**, then **Developer ID Application** (not
   "Developer ID Installer"), and click **Continue**. If you cannot choose it, you are not
   signed in as the Account Holder of the team.
6. For **Profile Type**, choose **G2 Sub-CA (Xcode 11.4.1 or later)** and click
   **Continue**.
7. **Choose File**, pick the `.certSigningRequest` file from step 3, click **Continue**,
   then **Download**. You get `developerID_application.cer`.
8. Double-click the `.cer` file. Keychain Access adds it to the **login** keychain, where
   it pairs with the private key from step 3.
9. Run `security find-identity -v -p codesigning` again: it now lists **Developer ID
   Application: *your name* (*TEAM ID*)**.

(Xcode can do steps 1 to 8 for you: **Xcode → Settings → Accounts →** your Apple ID
**→** your team **→ Manage Certificates… → + → Developer ID Application**.)

## 2. Export it as a .p12

1. In Keychain Access, select the **login** keychain and the **My Certificates** tab.
2. Find **Developer ID Application: *your name* (*TEAM ID*)** (not "Apple Development")
   and click the triangle next to it: a private key must be underneath. If there is none,
   the certificate was made on another Mac; export it from there, or make a new one.
3. Right-click the certificate (not the key), choose **Export**, pick the format
   **Personal Information Exchange (.p12)**, save it as `DeveloperID.p12`, and give
   it a strong password.
4. Check the file before you use it; it asks for that password and must print a subject
   with `Developer ID Application`:

   ```bash
   openssl pkcs12 -in DeveloperID.p12 -nokeys | grep -i subject
   ```

   If `openssl` cannot read the file (an "unsupported algorithm" error), skip this check:
   a GitHub run with the new secret also says what the `.p12` holds (see the end of this
   page).

## 3. Create an App Store Connect API key for notarization

1. Go to [appstoreconnect.apple.com](https://appstoreconnect.apple.com), then **Users
   and Access → Integrations → App Store Connect API → Team Keys**. The first time,
   click **Request Access** and accept the terms.
2. Click **+** (or **Generate API Key**), name it `Briss notarization`, give it the
   **Developer** access, and click **Generate**.
3. Click **Download** next to the new key. Apple lets you download it **once**; you get
   `AuthKey_XXXXXXXXXX.p8`, where the Xs are the **Key ID** shown in its row. Note that
   and the **Issuer ID** above the table.

The Key ID and the Issuer ID only name the key; notarizing needs the `.p8` file itself.
If you made a key before and no longer have its file, Apple cannot send it again:
**Revoke** that key on the same page, make a new one as above, and use the new Key ID.
The Issuer ID stays the same.

## 4. Add the five secrets to GitHub

On GitHub, open the repository, then **Settings → Secrets and variables → Actions →
New repository secret**, and add each one. For the two files, copy their base64 form
to the clipboard in Terminal and paste it as the value:

```bash
base64 -i DeveloperID.p12 | pbcopy
```

```bash
base64 -i AuthKey_XXXXXXXXXX.p8 | pbcopy
```

For `MACOS_NOTARY_KEY` you can also paste the `.p8` file's text as it is: open it in
TextEdit and copy all of it, from the line with BEGIN PRIVATE KEY to the line with END
PRIVATE KEY. The password, the Key ID and the Issuer ID are pasted as
they are. If a run warns that the builds are "signed but not notarized", it names the
secret that is missing or wrong. With the GitHub
command line you can set the files straight from Terminal instead:

```bash
base64 -i DeveloperID.p12 | gh secret set MACOS_SIGN_P12 --repo jarofclay/briss-go
```

```bash
base64 -i AuthKey_XXXXXXXXXX.p8 | gh secret set MACOS_NOTARY_KEY --repo jarofclay/briss-go
```

Afterwards keep `DeveloperID.p12` and the `.p8` file in a password manager, or delete
them; GitHub has what the builds need.

## 5. Try it without releasing anything

In the repository's **Actions** tab:

- **GUI → Run workflow** (leave `release_tag` empty) builds the app; the macOS job
  should say `Developer ID certificate ready`, `Signed ... with the Developer ID` and
  `Notarized ...; Gatekeeper accepts it.` for both Macs. The zips are under the run's
  **Artifacts**.
- **Release → Run workflow** builds a snapshot of the command line (nothing is
  released); the log shows the same lines for `briss_darwin_*`, followed by
  `the PDF engine runs ... under its signature`. The archives are under **Artifacts**
  as `briss-snapshot`.

On a Mac, a downloaded and unzipped app should now open without any warning, and

```bash
spctl --assess --type execute -vv /Applications/Briss.app
```

should answer `accepted` with `source=Notarized Developer ID`. For the command line,
`codesign -dv --verbose=2 briss` lists `Authority=Developer ID Application: ...` and a
`Runtime Version`.

## 6. Release

Push the next `v*` tag as before. `release.yml` (which now runs on a Mac) signs and
notarizes the two macOS command-line programs before it packs them, and `gui.yml` does
the same for the two apps before it attaches them. Once a signed release is out,
update **First start on a Mac** in the README and the release notes: the Open Anyway
steps are then only needed for older downloads.

## On your own Mac

Once the certificate from step 1 is in your login keychain, you can sign and notarize a
build on your Mac without GitHub. Save the notary key in the keychain once (it asks for
the key's file, its ID and the Issuer ID from step 3):

```bash
xcrun notarytool store-credentials briss-notary --key AuthKey_XXXXXXXXXX.p8 --key-id XXXXXXXXXX --issuer YOUR-ISSUER-ID
```

Then build with your certificate's name as Keychain Access shows it:

```bash
MACOS_SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" MACOS_NOTARY_PROFILE=briss-notary scripts/macos/build-app.sh
```

`dist/Briss-macos-apple-silicon.zip` is then signed, notarized and stapled, ready to
pass on (`scripts/macos/build-app.sh amd64` makes the Intel one). Without
`MACOS_NOTARY_PROFILE` it is signed only; with neither variable it gets an ad-hoc
signature, which is enough on the Mac that built it. The command line is signed and
notarized the same way:

```bash
go build -o briss ./cmd/briss && MACOS_SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)" MACOS_NOTARY_PROFILE=briss-notary scripts/macos/sign.sh briss
```

## When GitHub has no Mac free

GitHub's `macos-latest` machines are sometimes all busy, and a job waits and is then
cancelled ("not acquired by Runner"). The workflows take another macOS runner type from
the repository variable `MACOS_RUNNER`: **Settings → Secrets and variables → Actions →
Variables → New repository variable**, name `MACOS_RUNNER`, value for example
`macos-15`, `macos-14` or `macos-15-intel` (the [runner
list](https://docs.github.com/en/actions/using-github-hosted-runners/using-github-hosted-runners/about-github-hosted-runners#standard-github-hosted-runners-for-public-repositories)
names the current ones). Delete the variable to go back to `macos-latest`. Or build on
your own Mac as above.

## How it works, and what to watch

- `scripts/macos/signing-setup.sh` puts the certificate into a throwaway keychain and
  the key into a file on the runner; `scripts/macos/sign.sh` signs with the hardened
  runtime and a secure timestamp, sends a zip to Apple's notary service with
  `notarytool`, waits for the answer, and staples the ticket to the app (a bare
  command-line program cannot carry one; macOS fetches it online).
- The signature includes one exception to the hardened runtime,
  `com.apple.security.cs.allow-unsigned-executable-memory`
  (`scripts/macos/entitlements.plist`): the PDF engine compiles PDFium from
  WebAssembly to machine code while it runs, the case Apple names this entitlement
  for. Every GUI build runs the engine under an ad-hoc signature with the same
  options, and every signed release runs the signed Apple-silicon program once
  (`scripts/macos/check-engine.sh`). GitHub's Macs also ran it without the
  entitlement, so they may enforce less than a user's Mac: the first signed download,
  opened on a real Mac and used on a PDF, is the real test.
- If a run warns `MACOS_SIGN_P12 holds no valid Developer ID Application certificate`,
  the macOS builds went on with the ad-hoc signature, and the log of the step "Set up
  Developer ID signing" lists what the `.p12` holds, with your name left out. Nothing under "Signing
  identities" means the private key is missing: export again from **My Certificates**,
  where the key shows under the certificate (step 2). Another kind, such as "Apple
  Development" or "Developer ID Installer", means the wrong certificate type (step 1).
  `CSSMERR_TP_NOT_TRUSTED` means its issuer is missing; the script installs Apple's
  Developer ID intermediates from apple.com, so that points to a certificate from
  another issuer.
- Signing runs only for tags and manual runs, never for pull requests, and GitHub never
  passes secrets to workflows started from forks. Anyone who can push to this
  repository can start a workflow that uses them, so keep write access to people you
  trust.
- If the certificate or the key leaks, revoke the certificate under **Certificates** on
  developer.apple.com and the key under **Team Keys** in App Store Connect, then make
  new ones and replace the secrets. A Developer ID certificate lasts five years.
- Windows signing (for SmartScreen) is a separate matter and needs a code-signing
  certificate from a Windows certificate authority; it is not set up.
