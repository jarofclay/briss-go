# Briss 2.0 
![Java CI with Gradle](https://github.com/mbaeuerle/Briss-2.0/actions/workflows/gradle.yml/badge.svg)

Briss is a small application to crop PDF files. It is useful for example to crop whitespaces at the edeges so text is rendered bigger on small screens like eInk displays or tablet PCs.

> **Rewrite in progress:** see [PLAN.md](PLAN.md) for the plan to rewrite Briss in Go with AI-assisted crop detection, [HANDOFF.md](HANDOFF.md) for the current state and next steps, [docs/decisions](docs/decisions/README.md) for the decisions taken, and [spike/RESULTS.md](spike/RESULTS.md) for the week-1 measurements. The Java version moved to [legacy-java/](legacy-java/).

It helps the user to decide what should be cropped by creating an overlay of similar pages, e.g. all pages within a PDF
 having the same size or orientation. Even and odd pages are separated too.

Version 2.0 is intended to be a GUI Update for the Briss PDF cropping tool.
It is based on Briss 0.9 which is located at sourceforge: http://sourceforge.net/projects/briss/
Note that version 2.0 in this repository is developed independently of the original author of 0.9.

Briss is running on Windows, MacOS and Linux and works with Java 8 and above.

## Installation

Currently Briss 2.0 is in alpha therefore some features are still missing.
If you want to give it a try you can download the pre release from https://github.com/mbaeuerle/Briss-2.0/releases.
Starting with v2.0-alpha-5 the whole application including the dependencies is bundled in one Java jar file.

## Usage
You can run the application by double-clicking on `./lib/Briss-2.0-all.jar` or executing the following command in terminal:

```
./bin/Briss-2.0
```
or
```
./bin/Briss-2.0 cropthis.pdf
```

## Commandline

If you prefer command line and trust the basic automatic detection algorithm
use it this way:

```
./bin/Briss-2.0 -s [SOURCEFILE] [-d [DESTINATIONFILE]]
```
Example:
```
./bin/Briss-2.0 -s dogeatdog.pdf -d dogcrop.pdf
./bin/Briss-2.0 -s dogeatdog.pdf
```
the second line will create the cropped pdf into `dogeatdog_cropped.pdf`

To split according to columns/rows, respectively use the `--split-col` and `--split-row` arguments. For example:
```
./bin/Briss-2.0 -s dogeatdog.pdf -d dogcrop.pdf --split-col
```

Splitting columns will try to split the pdf into two columns. Splitting rows will try to split the pdf into two parts
by cutting pages in half.

## Images

Startscreen with drag and drop support:
![Image of BRISS 2.0 Startscreen](img/startScreen.png)
Cropping view:
![Image of BRISS 2.0 Cropping View](img/croppingView.png)

## Improvements done in Briss 2.0
- Small refinements on gui which improve the workflow
- Better file chooser than provided by swing
- Added support for drag and drop
- Fixed an issue with the image preview showing nothing when all pages look mostly the same

## Build instructions

### Go rewrite (work in progress)
The Go version lives at the repository root. It needs only Go (1.26 or newer, which
`go` downloads automatically); PDFium runs as WebAssembly, so there is no cgo and no
system library to install.

```
go build -o briss ./cmd/briss
```

```
briss crop book.pdf                                  # detect and write book_cropped.pdf
briss crop book.pdf -o small.pdf --margin 3pt --split-col
briss detect book.pdf --save-crops book.crops.json   # save the rectangles, edit them
briss apply book.pdf --crops book.crops.json         # crop with the edited rectangles
briss preview book.pdf                               # overlay PNGs with the rectangles drawn
briss bench --dataset labels/ --report bench.md      # score against labelled PDFs (docs/datasets.md)
briss -s dogeatdog.pdf -d dogcrop.pdf --split-col    # the Briss 2.0 command line still works
```

`briss COMMAND --help` lists every option. `--detector auto` picks the scan detector
(`cv`, page by page, splits two-page sheets at the gutter) for scanned PDFs and the
legacy auto-crop for the rest; the graphical editor, the structural detector for
born-digital PDFs and the AI-assisted detectors come in later phases. The
first run compiles the PDF engine for a few seconds and caches the result in your user
cache directory. Run the tests with `go test ./...`.

### Legacy Java version
The Java Briss 2.0 moved to `legacy-java/`. It needs a JDK from version 11 up to 19:

```
cd legacy-java
./gradlew shadowDistZip
```

You can find the built version in `legacy-java/build/distributions`.

## Libraries used
 * This software uses two libraries to render and crop PDF files: 
  * itext (AGPLv3) http://itextpdf.com/ 
  * Apache PDFBox (Apache-2.0) https://pdfbox.apache.org/
  * JavaFX (GPLv2) https://openjfx.io
  
