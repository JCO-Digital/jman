package wpcli

import "testing"

func TestParseLowestAdminUserID(t *testing.T) {
	cases := []struct {
		name    string
		output  string
		want    int
		wantErr bool
	}{
		{
			name:   "single admin",
			output: `[{"ID":"5"}]`,
			want:   5,
		},
		{
			name:   "picks lowest ID regardless of order",
			output: `[{"ID":"9"},{"ID":"2"},{"ID":"14"}]`,
			want:   2,
		},
		{
			name:   "leading wp-cli notice before the JSON array",
			output: "Some warning notice\n" + `[{"ID":"3"}]`,
			want:   3,
		},
		{
			name:    "empty array",
			output:  `[]`,
			wantErr: true,
		},
		{
			name:    "no JSON array",
			output:  ``,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseLowestAdminUserID(tc.output)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got id %d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}
