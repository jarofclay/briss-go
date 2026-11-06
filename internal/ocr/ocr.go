// Package ocr recognises the words on scanned pages with Tesseract, an
// external program the user installs (https://tesseract-ocr.github.io), so
// briss can lay an invisible, selectable text layer over the scan.
package ocr

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// ErrNotInstalled means no tesseract program was found.
var ErrNotInstalled = errors.New("ocr: Tesseract is not installed (https://tesseract-ocr.github.io/tessdoc/Installation.html)")

// Word is one recognised word with its box in pixels of the page image.
type Word struct {
	Text           string
	X0, Y0, X1, Y1 int
	Conf           float64
}

// candidates are places Tesseract is installed to when it is not on PATH;
// apps started from a desktop often get a shorter PATH than a terminal.
func candidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/opt/homebrew/bin/tesseract", "/usr/local/bin/tesseract", "/opt/local/bin/tesseract"}
	case "windows":
		var out []string
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
			if d := os.Getenv(env); d != "" {
				out = append(out, filepath.Join(d, "Tesseract-OCR", "tesseract.exe"), filepath.Join(d, "Programs", "Tesseract-OCR", "tesseract.exe"))
			}
		}
		return out
	default:
		return []string{"/usr/bin/tesseract", "/usr/local/bin/tesseract", "/snap/bin/tesseract"}
	}
}

// Find returns the path of the tesseract program: $BRISS_TESSERACT, then
// PATH, then the usual install locations.
func Find() (string, error) {
	if p := os.Getenv("BRISS_TESSERACT"); p != "" {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	if p, err := exec.LookPath("tesseract"); err == nil {
		return p, nil
	}
	for _, p := range candidates() {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", ErrNotInstalled
}

// Languages lists the installed language packs.
func Languages(ctx context.Context, bin string) ([]string, error) {
	out, err := exec.CommandContext(ctx, bin, "--list-langs").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ocr: %s --list-langs: %w", bin, err)
	}
	var langs []string
	for _, l := range strings.Split(string(out), "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.Contains(l, " ") && !strings.HasSuffix(l, ":") && l != "osd" {
			langs = append(langs, l)
		}
	}
	return langs, nil
}

// Recognize runs Tesseract on img and returns its words. lang is a
// Tesseract language list such as "eng" or "eng+deu" (empty: "eng").
func Recognize(ctx context.Context, bin string, img image.Image, lang string) ([]Word, error) {
	if lang == "" {
		lang = "eng"
	}
	dir, err := os.MkdirTemp("", "briss-ocr-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	in := filepath.Join(dir, "page.png")
	f, err := os.Create(in)
	if err != nil {
		return nil, err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return nil, err
	}
	f.Close()
	cmd := exec.CommandContext(ctx, bin, in, "stdout", "-l", lang, "--psm", "3", "tsv")
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1") // briss runs several pages at once
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ocr: tesseract: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return ParseTSV(out)
}

// ParseTSV reads Tesseract's TSV output and returns the words (level 5)
// with text and a non-negative confidence.
func ParseTSV(b []byte) ([]Word, error) {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<16), 1<<24)
	var words []Word
	first := true
	for sc.Scan() {
		line := sc.Text()
		if first {
			first = false
			if strings.HasPrefix(line, "level") {
				continue
			}
		}
		f := strings.Split(line, "\t")
		if len(f) < 12 || f[0] != "5" {
			continue
		}
		text := strings.TrimSpace(f[11])
		conf, _ := strconv.ParseFloat(f[10], 64)
		if text == "" || conf < 0 {
			continue
		}
		n := make([]int, 4)
		for i := range n {
			v, err := strconv.Atoi(f[6+i])
			if err != nil {
				return nil, fmt.Errorf("ocr: bad TSV line %q", line)
			}
			n[i] = v
		}
		words = append(words, Word{Text: text, X0: n[0], Y0: n[1], X1: n[0] + n[2], Y1: n[1] + n[3], Conf: conf})
	}
	return words, sc.Err()
}
