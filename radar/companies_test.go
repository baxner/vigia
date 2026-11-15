package radar

import "testing"

func TestParseCompaniesTSV(t *testing.T) {
	names := ParseCompaniesTSV("76\tApple, Inc.\r\n117\tSamsung Electronics Co. Ltd.\n")
	if len(names) != 2 || names[76] != "Apple, Inc." || names[117] != "Samsung Electronics Co. Ltd." {
		t.Fatalf("unexpected map: %+v", names)
	}

	// Blank lines, malformed lines, invalid IDs must not fail parsing.
	filtered := ParseCompaniesTSV("\nno-tab\n\tmissing-id\n224\t\nseis\tMicrosoft\n6\tMicrosoft\n369\tAmazon.com Services LLC\n   \r\n")
	if len(filtered) != 2 || filtered[6] != "Microsoft" || filtered[369] != "Amazon.com Services LLC" {
		t.Fatalf("unexpected filtered map: %+v", filtered)
	}
}

func TestEmbeddedCompanies(t *testing.T) {
	if count := KnownCompanyCount(); count < 4000 {
		t.Fatalf("KnownCompanyCount() = %d, want >= 4000", count)
	}
	cases := []struct {
		id    int
		full  string
		brand string
	}{
		{76, "Apple, Inc.", "Apple"},
		{117, "Samsung Electronics Co. Ltd.", "Samsung Electronics"},
		{224, "Google", "Google"},
		{6, "Microsoft", "Microsoft"},
		{911, "Xiaomi Inc.", "Xiaomi"},
	}
	for _, tc := range cases {
		if got := CompanyFullName(tc.id); got != tc.full {
			t.Errorf("CompanyFullName(%d) = %q, want %q", tc.id, got, tc.full)
		}
		if got := CompanyBrand(tc.id); got != tc.brand {
			t.Errorf("CompanyBrand(%d) = %q, want %q", tc.id, got, tc.brand)
		}
	}
	if got := CompanyFullName(-999); got != "" {
		t.Errorf("unknown full name = %q, want empty", got)
	}
	if got := CompanyBrand(-999); got != "" {
		t.Errorf("unknown brand = %q, want empty", got)
	}
}

func TestShortCompanyName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Apple, Inc.", "Apple"},
		{"Xiaomi Inc.", "Xiaomi"},
		{"Intel Corp.", "Intel"},
		{"Microsoft", "Microsoft"},
		{"Samsung Electronics Co. Ltd.", "Samsung Electronics"},
		{"Google", "Google"},
		{"FUSE BLUEJAY LIMITED", "FUSE BLUEJAY"},
		{"Jamco", "Jamco"},
		{"", ""},
		{"   ", ""},
		{"Acme GmbH", "Acme"},
		{"Foo Bar S.A.", "Foo Bar"},
	}
	for _, tc := range tests {
		if got := ShortCompanyName(tc.in); got != tc.want {
			t.Errorf("ShortCompanyName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
