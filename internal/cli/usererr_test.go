package cli

import (
	"errors"
	"testing"
)

func TestFormatUserError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "credentials",
			err:  errors.New("load AWS config: no valid credential sources"),
			want: "AWS credentials are not configured. Run: cloud-forge auth aws",
		},
		{
			name: "region",
			err:  errors.New("aws region is required; set --region"),
			want: "AWS region is required. Pass --region or set AWS_REGION / AWS_DEFAULT_REGION.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatUserError(tt.err); got != tt.want {
				t.Fatalf("formatUserError() = %q, want %q", got, tt.want)
			}
		})
	}
}
