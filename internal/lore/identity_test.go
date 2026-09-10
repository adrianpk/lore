package lore

import "testing"

type remoteNormalizationCase struct {
	name    string
	remote  string
	want    string
	wantErr bool
}

func TestNormalizeRepositoryRemote(t *testing.T) {
	t.Parallel()

	tests := []remoteNormalizationCase{
		{
			name:   "scp syntax",
			remote: "git@example.test:team/example.git",
			want:   "ssh://git@example.test/team/example.git",
		},
		{
			name:   "ssh password and suffix",
			remote: "ssh://git:password@EXAMPLE.test:2200//team/example",
			want:   "ssh://git@example.test:2200/team/example.git",
		},
		{
			name:   "https credentials and query",
			remote: "https://user:token@EXAMPLE.test/team//example.git?access_token=secret#fragment",
			want:   "https://example.test/team/example.git",
		},
		{
			name:    "missing path",
			remote:  "ssh://git@example.test",
			wantErr: true,
		},
		{
			name:    "unsupported scheme",
			remote:  "ftp://example.test/team/example.git",
			wantErr: true,
		},
		{
			name:    "local path",
			remote:  "/workspace/example",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assertNormalizedRepositoryRemote(t, test)
		})
	}
}

func assertNormalizedRepositoryRemote(t *testing.T, test remoteNormalizationCase) {
	t.Helper()

	got, err := NormalizeRepositoryRemote(test.remote)
	if test.wantErr {
		if err == nil {
			t.Fatal("expected an error")
		}

		return
	}

	if err != nil {
		t.Fatalf("NormalizeRepositoryRemote() error = %v", err)
	}

	if got != test.want {
		t.Fatalf("NormalizeRepositoryRemote() = %q, want %q", got, test.want)
	}
}

func TestNewRepositoryReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		root    string
		remote  string
		want    RepositoryReference
		wantErr bool
	}{
		{
			name:   "remote identity",
			root:   "/workspace/example",
			remote: "git@example.test:team/example",
			want: RepositoryReference{
				Name:     "example",
				Root:     "/workspace/example",
				Identity: "ssh://git@example.test/team/example.git",
				Remote:   "ssh://git@example.test/team/example.git",
			},
		},
		{
			name:    "local identity requires root commit",
			root:    "/workspace/local-example",
			wantErr: true,
		},
		{
			name:    "relative root",
			root:    "workspace/example",
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewRepositoryReference(test.root, test.remote)
			if test.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}

				return
			}

			if err != nil {
				t.Fatalf("NewRepositoryReference() error = %v", err)
			}

			if got != test.want {
				t.Fatalf("NewRepositoryReference() = %#v, want %#v", got, test.want)
			}
		})
	}
}
