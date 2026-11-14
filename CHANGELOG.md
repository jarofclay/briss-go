<!---
 * Copyright 2010 Gerhard Aigner
 * 
 * This file is part of BRISS.
 * 
 * BRISS is free software: you can redistribute it and/or modify it under the
 * terms of the GNU General Public License as published by the Free Software
 * Foundation, either version 3 of the License, or (at your option) any later
 * version.
 * 
 * BRISS is distributed in the hope that it will be useful, but WITHOUT ANY
 * WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
 * A PARTICULAR PURPOSE. See the GNU General Public License for more details.
 * 
 * You should have received a copy of the GNU General Public License along with
 * BRISS. If not, see http://www.gnu.org/licenses/.
--> 

# Change Log - BRISS
All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/)
and this project adheres to [Semantic Versioning](http://semver.org/).

## [Unreleased]

* The macOS app downloads are sealed with an ad-hoc signature, so macOS no longer says
  Briss "is damaged and can't be opened"; the first start still has to be allowed once
  with **Open Anyway** (README, "First start on a Mac"), because the app is not notarized.
* The GUI workflow can be run by hand with `release_tag` to rebuild the app downloads and
  replace them on an existing release.
* A logo: a page with its kept part framed by crop corners, on the app's dark tile. It is
  the icon of the macOS app (`Briss.icns`), of `Briss.exe` and `briss.exe` (Windows icon
  resources linked in by `go build`), of the app window, and ships as `briss.png` in the
  Linux download; the app header shows it next to the name. Sources in `assets/logo/`.
* Clicking the drop zone opens the system's own file picker instead of Fyne's dialog,
  which crashed on macOS: the Open panel on macOS (files and folders), the Explorer
  dialog on Windows, and the desktop's file chooser through the XDG portal on Linux,
  each with several files at once. Fyne's dialog remains as the fallback on Linux
  desktops without the portal.
* Developer ID signing and notarization of the macOS app and command line, done by the
  GUI and release workflows once the project's Apple secrets are set
  ([docs/macos-signing.md](docs/macos-signing.md)); until then the builds stay as
  before. The signature carries one hardened-runtime exception, for the code the PDF
  engine compiles while it runs.
  The release workflow now runs on a Mac and can be run by hand for a snapshot build.

* **Same page size** now gives every output page one size, the largest, left and right
  pages alike: a split two-page scan used to come out with its even and odd pages at
  different widths. A page shows more of its own margin where there is some and blank
  space where the page, or the facing page of a spread, begins (the writer makes the
  page larger than what it shows and clips the rest). Landscape pages, like a fold-out
  map, are sized among themselves so they do not widen the book's pages.
* The mode switch (Auto, Preview, Preview only) moved from the options panel to the middle
  of the header.
* The preview is one window for every file waiting for review. Page navigation (first,
  previous, next, last, and the page number) is in a bar at the top, next to buttons that
  step between the files (also `[` and `]`); the sheet and the result pages are headed
  **Source** and **Output**. **Save** is there in every mode, Preview only included, and
  moves on to the next file still waiting. Edits stay with each file while the window
  is open.
* `scripts/macos/build-app.sh` builds `Briss.app` and its zip on a Mac exactly as CI does
  (the GUI workflow now calls it); `scripts/macos/sign.sh` also notarizes with a
  `notarytool` keychain profile (`MACOS_NOTARY_PROFILE`), for signing on your own Mac.
  The repository variable `MACOS_RUNNER` picks another macOS runner type when
  `macos-latest` has none free.
* The site's favicon is the logo, with a PNG fallback and an Apple touch icon.

* Windows: the desktop app could not open any PDF ("could not instantiate webassembly
  module: GetFileType /dev/stdout: The handle is invalid"). Built without a console, it
  has no usable standard output, and the PDF engine handed wazero the invalid handles;
  it now passes plain writers. Tesseract also runs without flashing a console window
  for every page.
* The macOS signing setup installs Apple's Developer ID intermediate certificates and,
  when the `.p12` holds no usable Developer ID Application identity, warns, says what it
  holds (names left out) and lets the builds go on with the ad-hoc signature, so a
  wrong certificate cannot stop a release.

* Linux on ARM: a `Briss-linux-arm64.tar.gz` download, built on GitHub's ARM runners, next
  to the Intel and AMD one; on an ARM computer the x86-64 download failed with
  "qemu-x86_64-static: Could not open '/lib64/ld-linux-x86-64.so.2'". `install.sh` refuses
  the download that does not match the processor and names the right one.
* Linux: the download has an `install.sh` that puts Briss in the applications menu, with
  its icon and under "Open With" for PDFs (into `~/.local`, no administrator rights;
  `--uninstall` removes it), and a `README.txt`. The README explains the first start on
  Linux and how to put the command line on the `PATH`, since most file managers no
  longer start a program on a double-click.
* macOS notarization: the signing setup names the notary secret that is missing (the
  `.p8` key file was, while its Key ID and Issuer ID were set) and checks that
  `MACOS_NOTARY_KEY` holds a key, pasted as text or in base64. A notarization that Apple
  refuses, or that cannot be sent, now warns and leaves the build signed instead of
  stopping the release.
* The macOS signing guide tells the "Apple Development" certificate from the "Developer ID
  Application" one, shows how to check which ones a Mac has, and walks through a new
  certificate signing request step by step.

* The PDF engine no longer fails to start when its compilation cache cannot be used: on
  Windows, two processes writing the same cache entry at once (the app and the command
  line started together, or tests running side by side) made the rename fail with
  "Access is denied" and stopped the engine. It now starts without the cache, which only
  costs start-up time.

* macOS notarization in CI: the Key ID and Issuer ID secrets are cleaned of spaces and
  line breaks (and `AuthKey_…p8` is taken for its Key ID), and the signing setup asks
  Apple once whether it accepts the key before any build is sent. If Apple refuses it
  (the first tagged release got "401 Unauthenticated"), the warning says which value
  looks wrong. When Apple takes longer than the 30-minute wait (common for a team's first
  submissions), the warning now says the build was uploaded and names the submission to
  check, instead of blaming the secrets.

## [2.0-beta] - 2026-11-10
Briss rewritten in Go: a dark drop-and-go desktop app and a command line, with automatic
crop detection for scanned books (two-page spreads, binding shadows, page edges),
straightening, auto-rotation, same-size pages and a selectable OCR text layer. The Java
Briss 2.0 alpha stays in `legacy-java/`.

* [Issue-91] Migrate from JPedal to Apache PDFBox since the former has been removed from public repositories.
* The Java sources moved to `legacy-java/`; build there with `./gradlew shadowDistZip`.
* Go rewrite, phase 1: `cmd/briss` with the legacy command line (`-s`, `-d`, `-p`,
  `--split-col`, `--split-row`), PDFium as WebAssembly, pdfcpu writer that keeps outlines,
  named destinations and links on cropped and split pages. Differences from the Java
  version: pages cluster within a 10 pt size window instead of 20 pt bins and by their
  displayed size; a degenerate auto-crop keeps the page whole; `-d` accepts a directory.
* Go rewrite, phase 2: subcommands `crop`, `detect`, `apply`, `preview`, `bench` and
  `version`; crop definitions can be saved, edited and applied (`--save-crops`,
  `--crops`); `--exclude`, `--margin`, `--password-stdin`, `--dry-run` and `--json`;
  `briss bench` scores detectors against labelled PDFs (`docs/datasets.md`); release
  archives from goreleaser for Linux, macOS and Windows on amd64 and arm64; CI on all
  three systems and a gitleaks secret scan.
* Go rewrite, scan detector: `--detector cv` finds the content of every scanned page on
  its own (ink mask with binding shadows flattened, scanner borders, page edges and dust
  removed, text lines and figures grouped, page numbers and headings joined) and splits
  two-page sheets at the gutter; `auto` uses it for scanned PDFs. `briss preview --pages`
  draws on the pages themselves. `briss bench` runs detectors with the label's
  `keep_headers` setting and scores precomputed predictions with `--detectors dir:PATH`.
* Desktop app (`gui/`, branch `claude/gui`): a dark, minimal window with a drop zone;
  files are cropped automatically, or reviewed first in a preview window with draggable
  crop boxes and the resulting pages beside them. Options: mode (auto, preview, preview
  only), margin, same page size, OCR, straighten, keep headers. CI builds it for Linux,
  macOS and Windows.
* `crop` and `apply` take `--straighten` (scanned pages turned level), `--normalize`
  (one size for every output page of a kind) and `--ocr` / `--ocr-lang` (invisible text
  layer from Tesseract). Output files get the source file's permissions.
* Auto-rotate (on by default, `--auto-rotate=false` to turn off): scanned pages stored
  sideways or upside down are turned upright before detection, so a sideways two-page
  spread becomes its left and right page; undecided pages follow the rest of their
  group; turns are saved as `turn` in crop definitions and written as `/Rotate`. Tall
  sheets holding two pages one above the other are cut into a top and a bottom page.
* The desktop app asks for the password of protected PDFs (and again after a wrong
  one), has an Auto-rotate switch (on), and shows turned pages upright in the preview.
* Split options in the app: "Split two-page sheets" (on) and "Split each page" (Off,
  Columns, Rows). `--split-col` and `--split-row` now work on scans too (they were
  ignored by the scan detector): columns are cut between the text columns, rows between
  lines; `--no-auto-split` keeps two-page sheets together. Normalizing keeps the top and
  bottom parts of a page apart.
* `briss preview --crops FILE` draws a saved crop definition (for example a label) on the
  pages without detecting; `crop`, `apply` and `bench --report` create missing output
  folders. The README explains downloading, building and checking labels step by step.

## [2.0-alpha 5]

### Added
* [Issue-40](https://github.com/mbaeuerle/Briss-2.0/issues/40) Add file dialog for excluded pages again

### Changed
* JavaFX dependency removed and replaced by AWT FileDialog to clean up dependencies and increase Java version compatibility
* Release is now distributed as a single jar file containing all dependencies

## [2.0-alpha 4]

### Added

* [Issue-34](https://github.com/mbaeuerle/Briss-2.0/issues/34) Support for password-protected PDFs
* Crop rectangle is colored red if it will be removed after releasing mouse
* Resize handles on all corners and edges
* Mouse cursors resembling the actions which can be taken
* Deselect all crop rectangles on ESC

### Changed

* Redesign of the resize handles to make them more modern
* Allow for crop rectangle copy and paste on same panel
* [Issue-41](https://github.com/mbaeuerle/Briss-2.0/issues/41) Reverse crop rectangle selection order. Newest rectangle under cursor is selected.
* Update to Gradle 7.4
* Select crop rectangles with single click. Select multiple rectangles with `<shift>+click`.

### Fixed
* [Issue-5](https://github.com/mbaeuerle/Briss-2.0/issues/5) Crop rectangles cannot shrink to a value lower than zero any longer

## [2.0-alpha 3] - 2020-05-26

### Added
* Fallback overlay image algorithm to improve overlay when most pages look the same

### Changed
* Update to Gradle 6.3

## [2.0-alpha 2] - 2020-04-15

### Changed
* Code internal changes
* Java 8 now required because of JavaFX

## [2.0-alpha] - 2018-08-06

### Added
* Default size and position of window on startup
* Drag and drop support for loading PDFs
* Split crop rectangles horizontally or vertically
* Native file chooser for loading and storing PDFs

### Changed
* Increase progress bar update interval
* Improve visuals and arrangement of GUI
* Use Gradle as build tool

### Removed
* Popup to exclude pages after loading PDF
* Some crop functions like maximizing rectangles

## [0.9] - 2012-05-26
 * better handling of bookmarks
* library updates
* Rastislav Wartiak added a couple of user-interface/rectangle handling features
** rectangles have setable size and position
** keyboard bindings for actions
** show rectangle size of selected rectangles

## [0.0.13] - 1-20-4 
* Added automatically crop rectangle calculation
* Updated libraries (itext, jpedal) fixing several problems
* JP2000 support, finally!
* Added hotcorners(upper left and lower right) to resize existing crop rectangles
* Providing a single file as argument now directly loads it on startup of Briss 
     (can be handy if you have a shortcut defined like "open with")
* Command line cropping (at the moment just with automatic crop algorithm,
     see Readme for instructions)
* Better visualization of merged images
* Added possibility to reload pdfs for using other excluded pages while
     retaining crop rectangles
* Added drag&drop functionality
* Automatically clipping of crop rectangles to visible area
* Copy/paste of crop rectangles between clusters 	
* Hopefully fixed bug where on Mac the crop rectangles couldn't be deleted

## [0.0.12] - 2010-01-11
* Added Icon (thx to joão ziliotto)
* Reworked the GUI
* updated iText and JPedal libraries
* included a preview button

## [0.0.11] - 2010-10-10
* Exclude pages from merging
* Merged blocks are now sorted according to their lowest page number
* Faster scrolling (mouse)
* Fixed an issue where huge pages would cause BRISS to crash

## [0.0.10] - 2010-10-01
* fixed issure where a trim or bleed box would influence the cropping behaviour
* smaller file size through newer libraries
  * itext
  * jpedal 

## [0.0.9] - 2010-06-08
* Maximal pageheight of preview image is set to 600pixel
* last directory is being remembered 

## [0.0.8] - 2010-03-17
* Fixed a bug where bookmarks got lost during crop-transformation

## [0.0.7] - 2010-03-15
* Meta information is copied to the cropped file
* Small visual fix in progress bar
* Ghost rectangle shows the size of the last drawn crop rectangle (for people who want to have equally sized pages)
* Dragging of crop rectangles (press and hold the mouse button inside a crop rectangle)
* Single removal of crop rectangles by pressing the right mouse button inside the rectangle to be deleted
* Selected crop rectangles (=>ctrl+left mouse click into the rectangle) can be sized to the maximum of all selected widths/heights.

## [0.0.6] - 2010-05-13
* Added multiple column support (actually it's a multiple rectangle capability)
* Integrated a small description how to use the software.

## [0.0.5] - 2010-05-12
* Clustering of pages is now done with a small buffer regarding the size in order not to create to much clusters.
* Fixed a bug for pages not having a crop box... 

## [0.0.4] - 2010-05-11
* Switched to JPedal for Image rendering since its more robust

## [0.0.3] - 2010-05-11
* Resolved Bug:
* Crop rectangle height was limited to the same value as the width... 

## [0.0.2] - 2010-05-10
* Performance improvements
* Resolved Bugs:
   #2999011 - Handling of rotated PDFs is implemented
   #2999012 - Cropped rectangle is now limited to the preview image size 

## [0.0.1] - 2010-05-09
* Initial release
