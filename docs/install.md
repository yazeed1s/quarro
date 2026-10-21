# Installing Quarro

Quarro is free, and its downloads are not signed with a paid Apple or
Microsoft certificate. Your system will warn you the first time you open
it. The steps below show exactly what you will see and how to get past it.
If you see a warning that isn't described here, please open an issue.

Download the file for your system from the
[releases page](https://github.com/yazeed1s/quarro/releases).

<!-- release-notes:start -->
## Installing

| System | File |
| --- | --- |
| macOS 12+ (Apple Silicon and Intel) | `Quarro-<version>-universal.dmg` |
| Windows 10/11 (64-bit) | `Quarro-<version>-amd64-setup.exe` |
| Linux, any distribution | `Quarro-<version>-x86_64.AppImage` |
| Ubuntu 24.04+, Debian 13+ | `quarro_<version>_amd64.deb` |
| Fedora and other RPM distributions | `quarro-<version>.x86_64.rpm` |

Quarro's builds are free and unsigned, so macOS and Windows warn on first
launch. That's expected; the steps below get past it.

### Check the download (optional)

Each release includes `SHA256SUMS`. Download it next to your file, then:

- Linux: `sha256sum -c SHA256SUMS --ignore-missing`
- macOS: `shasum -a 256 -c SHA256SUMS --ignore-missing`
- Windows (PowerShell): `Get-FileHash .\Quarro-<version>-amd64-setup.exe -Algorithm SHA256`,
  then compare the hash with the matching line in `SHA256SUMS`.

A match means the file arrived intact and is the one listed on the
release page. It can't prove the release page itself wasn't tampered
with, because both come from the same place.

### macOS

1. Open the `.dmg` and drag **Quarro** into **Applications**.
2. Open it from Applications. macOS says it can't verify that the app is
   free of malware (the exact wording depends on your macOS version).
   - **macOS 15 Sequoia or later:** click **Done**, then open **System
     Settings → Privacy & Security**. Scroll down to the message about
     Quarro being blocked, click **Open Anyway**, and confirm with your
     password or Touch ID.
   - **macOS 12–14:** Control-click (or right-click) **Quarro** in
     Applications, choose **Open**, then click **Open** in the dialog.
   - Or, in Terminal: `xattr -dr com.apple.quarantine /Applications/Quarro.app`
3. You do this once per version; after an update macOS asks again.

If macOS says the app **"is damaged and can't be opened"**, that isn't the
normal warning. Please report it rather than working around it.

Saved connection passwords are kept in your login Keychain and stay there
across updates.

### Windows

1. Your browser may warn that the file isn't commonly downloaded. In Edge,
   open the **…** menu on the download, choose **Keep**, then **Show more →
   Keep anyway**.
2. Run the installer. If Windows shows **"Windows protected your PC"**,
   click **More info**, then **Run anyway**. The publisher shows as
   "Unknown publisher" because the installer isn't signed.
3. Quarro installs for your user account only, so it doesn't need
   administrator rights.

Occasionally an antivirus product flags unsigned programs by mistake. If
that happens, please open an issue with the product's name.

### Linux

**AppImage** (any distribution):

```sh
chmod +x Quarro-<version>-x86_64.AppImage
./Quarro-<version>-x86_64.AppImage
```

AppImages need FUSE 2. If it doesn't start, install it
(`sudo apt install libfuse2t64` on Ubuntu 24.04+, `libfuse2` on older
releases), or run it with `--appimage-extract-and-run`.

**Debian / Ubuntu:** `sudo apt install ./quarro_<version>_amd64.deb`

**Fedora / RPM:** `sudo dnf install ./quarro-<version>.x86_64.rpm`. If your
system enforces signatures on local packages, add `--nogpgcheck`.

The `.deb` and `.rpm` use your system's GTK 4 and WebKitGTK 6.0, which
Ubuntu 24.04+, Debian 13+, and current Fedora provide. On older
distributions, use the AppImage.

To keep saved passwords between sessions, Quarro needs a keyring service
(GNOME Keyring or KWallet; most desktops have one). Without one, Quarro
tells you that passwords won't be remembered after you close it.
<!-- release-notes:end -->
