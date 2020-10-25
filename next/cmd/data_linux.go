package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/twpayne/chezmoi/next/internal/chezmoi"
)

func getKernelInfo(system chezmoi.System) (map[string]string, error) {
	const procSysKernel = "/proc/sys/kernel"

	info, err := system.Stat(procSysKernel)
	switch {
	case os.IsNotExist(err):
		return nil, nil
	case os.IsPermission(err):
		return nil, nil
	case !info.Mode().IsDir():
		return nil, nil
	}

	kernelInfo := make(map[string]string)
	for _, filename := range []string{
		"osrelease",
		"ostype",
		"version",
	} {
		data, err := system.ReadFile(filepath.Join(procSysKernel, filename))
		switch {
		case os.IsNotExist(err):
			continue
		case os.IsPermission(err):
			continue
		case err != nil:
			return nil, err
		}
		kernelInfo[filename] = string(bytes.TrimSpace(data))
	}
	return kernelInfo, nil
}

// getOSRelease returns the operating system identification data as defined by
// https://www.freedesktop.org/software/systemd/man/os-release.html.
func getOSRelease(system chezmoi.System) (map[string]string, error) {
	for _, filename := range []string{
		"/usr/lib/os-release",
		"/etc/os-release",
	} {
		data, err := system.ReadFile(filename)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		m, err := parseOSRelease(bytes.NewBuffer(data))
		if err != nil {
			return nil, err
		}
		return m, nil
	}
	return nil, os.ErrNotExist
}

// maybeUnquote removes quotation marks around s.
func maybeUnquote(s string) string {
	// Try to unquote.
	if s, err := strconv.Unquote(s); err == nil {
		return s
	}
	// Otherwise return s, unchanged.
	return s
}

// parseOSRelease parses operating system identification data from r as defined
// by https://www.freedesktop.org/software/systemd/man/os-release.html.
func parseOSRelease(r io.Reader) (map[string]string, error) {
	result := make(map[string]string)
	s := bufio.NewScanner(r)
	for s.Scan() {
		// trim all leading whitespace, but not necessarily trailing whitespace
		token := strings.TrimLeftFunc(s.Text(), unicode.IsSpace)
		// if the line is empty or starts with #, skip
		if len(token) == 0 || token[0] == '#' {
			continue
		}
		fields := strings.SplitN(token, "=", 2)
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s: parse error", token)
		}
		key := fields[0]
		value := maybeUnquote(fields[1])
		result[key] = value
	}
	return result, s.Err()
}
