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

## 1. Create the Developer ID Application certificate

1. On your Mac, open **Keychain Access**. From its menu choose **Keychain Access →
   Certificate Assistant → Request a Certificate From a Certificate Authority**. Enter
   your email address and your name, choose **Saved to disk**, and continue. This saves
   `CertificateSigningRequest.certSigningRequest` and puts a new private key in your
   login keychain.
2. Go to [developer.apple.com/account](https://developer.apple.com/account), then
   **Certificates, IDs & Profiles → Certificates**, and click **+**.
3. Choose **Developer ID Application** (not "Apple Development" or "Mac App
   Distribution"), continue, pick the **G2 Sub-CA** profile, upload the request file
   from step 1, and click **Download**.
4. Double-click the downloaded `developerID_application.cer`; it joins your login
   keychain next to the private key.

(Xcode can do the same: **Settings → Accounts →** your team **→ Manage Certificates
→ + → Developer ID Application**.)

## 2. Export it as a .p12

1. In Keychain Access, select the **login** keychain and the **My Certificates** tab.
2. Find **Developer ID Application: *your name* (*TEAM ID*)** and click the triangle
   next to it: a private key must be underneath. If there is none, the certificate was
   made on another Mac; export it from there, or make a new one.
3. Right-click the certificate (not the key), choose **Export**, pick the format
   **Personal Information Exchange (.p12)**, save it as `DeveloperID.p12`, and give
   it a strong password.

## 3. Create an App Store Connect API key for notarization

1. Go to [appstoreconnect.apple.com](https://appstoreconnect.apple.com), then **Users
   and Access → Integrations → App Store Connect API → Team Keys**. The first time,
   click **Request Access** and accept the terms.
2. Click **+** (or **Generate API Key**), name it `Briss notarization`, give it the
   **Developer** access, and click **Generate**.
3. Click **Download** next to the new key. Apple lets you download it **once**; you get
   `AuthKey_XXXXXXXXXX.p8`. Note the **Key ID** in its row and the **Issuer ID** above
   the table.

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

The password, the Key ID and the Issuer ID are pasted as they are. With the GitHub
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
- If the log says `MACOS_SIGN_P12 holds no valid Developer ID Application certificate`,
  it lists what the `.p12` holds, with your name left out. Nothing under "Signing
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
