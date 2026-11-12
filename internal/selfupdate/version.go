// Package selfupdate finds new Mypack releases and works out how this copy
// of Mypack is updated: by itself (MyGo's signed updater) or by the package
// manager that installed it. It has no UI; internal/desktop/app shows it.
package selfupdate

import (
	"strconv"
	"strings"
)

// Version is a parsed semantic version, "1.2.3" or "1.2.3-beta.1".
type Version struct {
	Major, Minor, Patch int
	Pre                 []string // dot-separated prerelease identifiers
}

// ParseVersion parses "v1.2.3", "1.2.3" or "1.2.3-rc.1" (build metadata
// after "+" is ignored). ok is false for anything else.
func ParseVersion(s string) (v Version, ok bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	core, pre, hasPre := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, false
	}
	nums := [3]int{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || p == "" || (len(p) > 1 && p[0] == '0') {
			return Version{}, false
		}
		nums[i] = n
	}
	v = Version{Major: nums[0], Minor: nums[1], Patch: nums[2]}
	if hasPre {
		if pre == "" {
			return Version{}, false
		}
		for _, id := range strings.Split(pre, ".") {
			if id == "" {
				return Version{}, false
			}
			v.Pre = append(v.Pre, id)
		}
	}
	return v, true
}

// IsPrerelease reports whether v has a prerelease part.
func (v Version) IsPrerelease() bool { return len(v.Pre) > 0 }

// String formats v without a leading "v".
func (v Version) String() string {
	s := strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
	if len(v.Pre) > 0 {
		s += "-" + strings.Join(v.Pre, ".")
	}
	return s
}

// Cmp orders versions as semantic versioning does: 1.3.0-beta.2 before
// 1.3.0-beta.10 before 1.3.0.
func (v Version) Cmp(w Version) int {
	for _, d := range [3]int{v.Major - w.Major, v.Minor - w.Minor, v.Patch - w.Patch} {
		if d != 0 {
			return sign(d)
		}
	}
	switch {
	case len(v.Pre) == 0 && len(w.Pre) == 0:
		return 0
	case len(v.Pre) == 0:
		return 1
	case len(w.Pre) == 0:
		return -1
	}
	for i := 0; i < len(v.Pre) && i < len(w.Pre); i++ {
		if c := cmpIdent(v.Pre[i], w.Pre[i]); c != 0 {
			return c
		}
	}
	return sign(len(v.Pre) - len(w.Pre))
}

func cmpIdent(a, b string) int {
	an, aerr := strconv.Atoi(a)
	bn, berr := strconv.Atoi(b)
	switch {
	case aerr == nil && berr == nil:
		return sign(an - bn)
	case aerr == nil:
		return -1 // numeric identifiers sort before alphanumeric ones
	case berr == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

// Compare compares two version strings; one that does not parse sorts
// before every one that does.
func Compare(a, b string) int {
	va, oka := ParseVersion(a)
	vb, okb := ParseVersion(b)
	switch {
	case !oka && !okb:
		return 0
	case !oka:
		return -1
	case !okb:
		return 1
	}
	return va.Cmp(vb)
}

// IsNewer reports whether candidate is newer than current. A current
// version that does not parse (an unstamped build) is never offered one.
func IsNewer(candidate, current string) bool {
	if _, ok := ParseVersion(current); !ok {
		return false
	}
	return Compare(candidate, current) > 0
}
