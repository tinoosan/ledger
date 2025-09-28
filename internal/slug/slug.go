package slug

import (
	"regexp"
	"strings"
)

var reSlug = regexp.MustCompile(`^[a-z0-9_]{2,40}$`)

// IsSlug returns true if s matches ^[a-z0-9_]{2,40}$
func IsSlug(s string) bool {
	return reSlug.MatchString(s)
}

// Slugify converts s to a slug: lowercase, non [a-z0-9_] -> '_', collapse repeats, trim to 40, and trim leading/trailing '_'.
func Slugify(s string) string {
	if s == "" {
		return s
	}
	out := make([]rune, 0, len(s))
	prevUnderscore := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			if r == '_' {
				if prevUnderscore {
					continue
				}
				prevUnderscore = true
			} else {
				prevUnderscore = false
			}
			out = append(out, r)
		} else {
			if !prevUnderscore {
				out = append(out, '_')
				prevUnderscore = true
			}
		}
		if len(out) >= 40 {
			break
		}
	}
	// trim leading/trailing underscores
	res := strings.Trim(string(out), "_")
	return res
}
