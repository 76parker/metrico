package buildinfo

import "testing"

func TestFormat(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		date     string
		commit   string
		expected string
	}{
		{
			name:     "provided values",
			version:  "v1.2.3",
			date:     "2026-10-19",
			commit:   "abc123",
			expected: "Build version: v1.2.3\nBuild date: 2026-10-19\nBuild commit: abc123\n",
		},
		{
			name:     "all values empty",
			expected: "Build version: N/A\nBuild date: N/A\nBuild commit: N/A\n",
		},
		{
			name:     "some values empty",
			version:  "v1.2.3",
			commit:   "abc123",
			expected: "Build version: v1.2.3\nBuild date: N/A\nBuild commit: abc123\n",
		},
		{
			name:     "version and commit empty",
			date:     "2026-10-19",
			expected: "Build version: N/A\nBuild date: 2026-10-19\nBuild commit: N/A\n",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if got := Format(testCase.version, testCase.date, testCase.commit); got != testCase.expected {
				t.Errorf("Format() = %q, want %q", got, testCase.expected)
			}
		})
	}
}
