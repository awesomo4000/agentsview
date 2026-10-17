package versionutil

import (
	"golang.org/x/mod/semver"
	"regexp"
	"strings"
)

func extractBaseSemver(v string) string {
	v = strings.TrimPrefix(v, "v")
	if len(v) == 0 || v[0] < '0' || v[0] > '9' {
		return ""
	}
	if !strings.Contains(v, ".") {
		return ""
	}
	if idx := strings.Index(v, "-"); idx > 0 {
		v = v[:idx]
	}
	return v
}

var gitDescribePattern = regexp.MustCompile(
	`-\d+-g[0-9a-f]+(-dirty)?$`,
)

// IsDevBuildVersion returns true if the version is a dev build.
func IsDevBuildVersion(v string) bool {
	v = strings.TrimPrefix(v, "v")
	if extractBaseSemver(v) == "" {
		return true
	}
	return gitDescribePattern.MatchString(v)
}

// IsNewer reports whether v1 is a semver release newer than v2. Dev builds
// and non-semver strings are not considered newer.
func IsNewer(v1, v2 string) bool {
	base1 := extractBaseSemver(v1)
	base2 := extractBaseSemver(v2)
	if base1 == "" || base2 == "" {
		return false
	}
	sv1 := normalizeSemver(v1)
	sv2 := normalizeSemver(v2)
	return semver.Compare(sv1, sv2) > 0
}

func isNewer(v1, v2 string) bool {
	return IsNewer(v1, v2)
}

var prereleaseNumericPattern = regexp.MustCompile(
	`^([A-Za-z]+)(\d+)$`,
)

func normalizeSemver(v string) string {
	v = strings.TrimPrefix(v, "v")
	if gitDescribePattern.MatchString(v) {
		v = gitDescribePattern.ReplaceAllString(v, "")
	}
	if idx := strings.Index(v, "-"); idx > 0 {
		base := v[:idx]
		prerelease := v[idx+1:]
		prerelease = normalizePrereleaseIdentifiers(prerelease)
		v = base + "-" + prerelease
	}
	return "v" + v
}

func normalizePrereleaseIdentifiers(prerelease string) string {
	parts := strings.Split(prerelease, ".")
	var result []string
	for _, part := range parts {
		matches := prereleaseNumericPattern.FindStringSubmatch(part)
		if matches != nil {
			letters, digits := matches[1], matches[2]
			if len(digits) > 1 && digits[0] == '0' {
				result = append(result, part)
			} else {
				result = append(result, letters, digits)
			}
		} else {
			result = append(result, part)
		}
	}
	return strings.Join(result, ".")
}
