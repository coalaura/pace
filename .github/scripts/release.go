// Copyright 2026 coalaura. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Release validates tags and packages the three-tool overlay. Run from the
// repository root with the official bootstrap Go, independently of cmd/go.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

var stableTag = regexp.MustCompile(`^pace[0-9]+\.[0-9]+\.[0-9]+$`)

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 3 && os.Args[1] == "version" {
		version, err := releaseVersion(os.Args[2])
		if err != nil {
			return err
		}

		fmt.Println(version)

		return nil
	}
	if len(os.Args) < 2 || os.Args[1] != "package" {
		return fmt.Errorf("usage: release version TAG | release package --tag TAG --goos OS --goarch ARCH --overlay DIR --output DIR")
	}

	flags := flag.NewFlagSet("package", flag.ContinueOnError)
	tag := flags.String("tag", "", "PACE release tag")
	goos := flags.String("goos", "", "target operating system")
	goarch := flags.String("goarch", "", "target architecture")
	overlay := flags.String("overlay", "", "directory containing the three signed tools")
	output := flags.String("output", "", "archive output directory")
	err := flags.Parse(os.Args[2:])
	if err != nil {
		return err
	}
	if flags.NArg() != 0 || *overlay == "" || *output == "" {
		return fmt.Errorf("package requires --overlay and --output, with no positional arguments")
	}
	if (*goos != "windows" && *goos != "linux" && *goos != "darwin") || (*goarch != "amd64" && *goarch != "arm64") {
		return fmt.Errorf("unsupported release target %s/%s", *goos, *goarch)
	}

	version, err := releaseVersion(*tag)
	if err != nil {
		return err
	}
	err = prepareOverlay(*overlay, *goos)
	if err != nil {
		return err
	}
	err = os.MkdirAll(*output, 0755)
	if err != nil {
		return err
	}

	extension := ".tar.gz"
	if *goos == "windows" {
		extension = ".zip"
	}
	archive := filepath.Join(*output, "pace-"+version+"-"+*goos+"-"+*goarch+extension)
	err = writeArchive(*overlay, archive, *goos == "windows")
	if err == nil {
		fmt.Println(archive)
	}
	return err
}

func releaseVersion(tag string) (string, error) {
	// Only exact Go-version release tags are supported.
	if !stableTag.MatchString(tag) {
		return "", fmt.Errorf("expected a stable PACE tag such as pace1.27.1, got %q", tag)
	}

	data, err := os.ReadFile("VERSION")
	if err != nil {
		return "", err
	}

	upstream, _, _ := strings.Cut(string(data), "\n")
	version := strings.TrimPrefix(tag, "pace")
	if strings.TrimSpace(upstream) != "go"+version {
		return "", fmt.Errorf("tag %s does not match VERSION %s", tag, upstream)
	}

	return version, nil
}

func prepareOverlay(overlay, goos string) error {
	suffix := ""
	if goos == "windows" {
		suffix = ".exe"
	}
	expected := []string{"bin/asmpe" + suffix, "bin/compilepe" + suffix, "bin/pace" + suffix}
	actual, err := overlayFiles(overlay)
	if err != nil {
		return err
	}
	if !slices.Equal(actual, expected) {
		return fmt.Errorf("expected only three signed tools in %s; found %v", overlay, actual)
	}

	names := []string{"README.md", "LICENSE", "PATENTS", "NOTICE"}
	for _, name := range names {
		err = copyFile(name, filepath.Join(overlay, "pace", name))
		if err != nil {
			return err
		}
	}

	// Conservatively collect command and standard-library vendor notices,
	// preserving source paths rather than maintaining a dependency license list.
	roots := []string{"src/cmd/vendor", "src/vendor"}
	for _, root := range roots {
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !isNotice(entry.Name()) {
				return nil
			}
			return copyFile(path, filepath.Join(overlay, "pace", "licenses", path))
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func writeArchive(overlay, archive string, windows bool) (err error) {
	files, err := overlayFiles(overlay)
	if err != nil {
		return err
	}
	output, err := os.Create(archive)
	if err != nil {
		return err
	}
	defer func() { err = closeOutput(output, err) }()

	var zipOutput *zip.Writer
	var tarOutput *tar.Writer
	if windows {
		zipOutput = zip.NewWriter(output)
		defer func() { err = closeOutput(zipOutput, err) }()
	} else {
		compressed := gzip.NewWriter(output)
		defer func() { err = closeOutput(compressed, err) }()
		tarOutput = tar.NewWriter(compressed)
		defer func() { err = closeOutput(tarOutput, err) }()
	}

	for _, name := range files {
		path := filepath.Join(overlay, filepath.FromSlash(name))
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		mode := fs.FileMode(0644)
		if strings.HasPrefix(name, "bin/") {
			mode = 0755
		}

		var destination io.Writer
		if windows {
			header := &zip.FileHeader{Name: name, Method: zip.Deflate}
			header.SetMode(mode)
			header.SetModTime(info.ModTime())
			destination, err = zipOutput.CreateHeader(header)
		} else {
			err = tarOutput.WriteHeader(&tar.Header{Name: name, Mode: int64(mode), Size: info.Size(), ModTime: info.ModTime()})
			destination = tarOutput
		}
		if err != nil {
			return err
		}
		err = copyTo(destination, path)
		if err != nil {
			return err
		}
	}
	return nil
}

func overlayFiles(root string) ([]string, error) {
	files := make([]string, 0, 64)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("overlay entry is not a regular file: %s", path)
		}
		name, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(name))
		return nil
	})
	slices.Sort(files)
	return files, err
}

func isNotice(name string) bool {
	name = strings.ToUpper(name)
	return strings.HasPrefix(name, "LICENSE") || strings.HasPrefix(name, "LICENCE") || strings.HasPrefix(name, "COPYING") ||
		strings.HasPrefix(name, "NOTICE") || strings.HasPrefix(name, "PATENTS") || strings.HasPrefix(name, "COPYRIGHT") ||
		strings.HasPrefix(name, "AUTHORS")
}

func copyFile(source, destination string) (err error) {
	err = os.MkdirAll(filepath.Dir(destination), 0755)
	if err != nil {
		return err
	}
	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	defer func() { err = closeOutput(output, err) }()
	return copyTo(output, source)
}

func copyTo(destination io.Writer, path string) error {
	source, err := os.Open(path)
	if err != nil {
		return err
	}
	_, err = io.Copy(destination, source)
	return closeOutput(source, err)
}

func closeOutput(output io.Closer, previous error) error {
	err := output.Close()
	if previous != nil {
		return previous
	}
	return err
}
