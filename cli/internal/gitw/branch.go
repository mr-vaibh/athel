package gitw

import "strings"

// maxSlug keeps branch names readable; the slug is cut at a word boundary.
const maxSlug = 40

// BranchName fills git.yaml's branch_name template (doc 04): {change_id} and {slug}, the
// change title in lower-case ASCII words joined by hyphens.
func BranchName(template, change, title string) string {
	return strings.NewReplacer("{change_id}", change, "{slug}", Slug(title)).Replace(template)
}

// Slug is title as lower-case ASCII letters and digits, words joined by "-", at most maxSlug
// characters, or "change" when nothing is left.
func Slug(title string) string {
	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	slug := ""
	for _, w := range words {
		next := w
		if slug != "" {
			next = slug + "-" + w
		}
		if len(next) > maxSlug {
			break
		}
		slug = next
	}
	if slug == "" {
		return "change"
	}
	return slug
}

// Prefix is the fixed start of a branch template, before its first placeholder.
func Prefix(template string) string {
	p, _, _ := strings.Cut(template, "{")
	return p
}
