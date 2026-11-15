package core

import "testing"

func TestNormalizeMAC(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"8:0:27:f0:fb:f9", "08:00:27:f0:fb:f9"},
		{"8-0-27-F0-FB-F9", "08:00:27:f0:fb:f9"},
		{"0:0:0:0:0:0", "00:00:00:00:00:00"},
		{"AA:BB:CC:DD:EE:FF", "aa:bb:cc:dd:ee:ff"},
		{"aa-bb-cc-dd-ee-ff", "aa:bb:cc:dd:ee:ff"},
		{"aabb.ccdd.eeff", "aa:bb:cc:dd:ee:ff"},
		{"AABB.CCDD.EEFF", "aa:bb:cc:dd:ee:ff"},
		{"aabb-ccdd-eeff", "aa:bb:cc:dd:ee:ff"},
		{"aabbccddeeff", "aa:bb:cc:dd:ee:ff"},
		{" 8:a:b:c:d:e ", "08:0a:0b:0c:0d:0e"},
		// Inválidas
		{"", ""},
		{"8:0:27:f0:fb", ""},
		{"8:0:27:f0:fb:f9:01", ""},
		{"8:0-27:f0:fb:f9", ""},
		{"012:34:56:78:9a:bc", ""},
		{"1:2:3:4:5:6:7:8:9:a:b:c", ""},
		{"zz:00:11:22:33:44", ""},
		{"aabb.ccdd.eef", ""},
		{"not-a-mac-address", ""},
	}
	for _, tc := range tests {
		if got := normalizeMAC(tc.in); got != tc.want {
			t.Errorf("normalizeMAC(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
