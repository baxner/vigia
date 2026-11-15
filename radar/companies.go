package radar

import (
	_ "embed"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

//go:embed bt_companies.tsv
var embeddedCompaniesTSV string

var (
	companiesOnce sync.Once
	companiesMap  map[int]string
)

func loadCompanies() map[int]string {
	companiesOnce.Do(func() {
		companiesMap = ParseCompaniesTSV(embeddedCompaniesTSV)
	})
	return companiesMap
}

// ParseCompaniesTSV parses "<decimal-id>\t<full name>" lines (tolerating CRLF,
// blank lines, and malformed rows) into a map of Bluetooth SIG company IDs.
func ParseCompaniesTSV(raw string) map[int]string {
	names := make(map[int]string)
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		tab := strings.IndexByte(line, '\t')
		if tab <= 0 {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(line[:tab]))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(line[tab+1:])
		if name != "" {
			names[id] = name
		}
	}
	return names
}

// KnownCompanyCount returns the number of Bluetooth SIG manufacturers loaded
// from the embedded bt_companies.tsv table.
func KnownCompanyCount() int {
	return len(loadCompanies())
}

// CompanyFullName returns the official Bluetooth SIG manufacturer name for id,
// or "" if id is not in the table.
func CompanyFullName(id int) string {
	return loadCompanies()[id]
}

// CompanyBrand returns a shortened display name for the Bluetooth SIG
// manufacturer id, or "" if id is unknown.
func CompanyBrand(id int) string {
	full := CompanyFullName(id)
	if full == "" {
		return ""
	}
	return ShortCompanyName(full)
}

var legalSuffixes = []string{
	"Incorporated", "Inc", "LLC", "L.L.C",
	"Limited", "Ltd",
	"Corporation", "Corp",
	"Company", "Co",
	"GmbH", "AG", "KG",
	"S.A.U", "S.A", "S.L", "S.p.A", "S.r.l",
	"B.V", "N.V",
	"A/S", "ApS",
	"Oy", "AB",
	"SAS", "SARL",
	"PLC",
	"Pty", "Pte",
	"d.o.o", "s.r.o", "K.K", "JSC", "UAB", "Lda",
}

var suffixPatterns = func() []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(legalSuffixes))
	for _, s := range legalSuffixes {
		re := regexp.MustCompile(`(?i)(,\s*|\s+)` + regexp.QuoteMeta(s) + `\.?\s*$`)
		patterns = append(patterns, re)
	}
	return patterns
}()

// ShortCompanyName trims up to 3 trailing corporate legal suffixes from
// fullName ("Apple, Inc." -> "Apple", "Samsung Electronics Co. Ltd." ->
// "Samsung Electronics").
func ShortCompanyName(fullName string) string {
	name := strings.TrimSpace(fullName)
	if name == "" {
		return ""
	}
	for attempts := 0; attempts < 3; attempts++ {
		next, changed := stripOneSuffix(name)
		if !changed {
			break
		}
		name = next
	}
	return name
}

func stripOneSuffix(name string) (string, bool) {
	for _, p := range suffixPatterns {
		if p.MatchString(name) {
			stripped := strings.TrimRight(p.ReplaceAllString(name, ""), " ,.")
			if strings.TrimSpace(stripped) != "" {
				return stripped, true
			}
		}
	}
	return name, false
}
